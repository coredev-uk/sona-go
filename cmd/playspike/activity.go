package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/activity"
	"github.com/coredev-uk/sona-go/internal/playback/coordinator"
	"github.com/coredev-uk/sona-go/internal/playback/native"
)

// runActivity proves play-activity reporting: it plays songID (or term's
// first hit) for listen (≥30 s), skips to nextID (or nextTerm's), stops,
// and polls Apple's Recently Played until the first song shows up at the
// top. With container ("album:1440…", "playlist:pl.…", "station:ra.…")
// the songs are queued from it, and it must also appear in Recently
// Played's containers.
func runActivity(log *slog.Logger, envFile, songID, term, nextID, nextTerm string, listen time.Duration, cdm, container string) error {
	var from coordinator.Container
	if container != "" {
		kind, id, ok := strings.Cut(container, ":")
		from = coordinator.Container{Kind: coordinator.ContainerKind(kind), ID: id}
		if !ok || id == "" || !from.Kind.Valid() {
			return fmt.Errorf("-container %q: want album:<id>, playlist:<id>, artist:<id> or station:<id>", container)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	api, sf, err := signIn(ctx, envFile)
	if err != nil {
		return err
	}
	first, err := resolve(ctx, api, sf.ID, songID, term)
	if err != nil {
		return err
	}
	second, err := resolve(ctx, api, sf.ID, nextID, nextTerm)
	if err != nil {
		return err
	}
	before, err := recent(ctx, api)
	if err != nil {
		return err
	}
	step("recently played before", "top", strings.Join(before[:min(5, len(before))], ", "))
	if from.ID != "" {
		got, err := recentContainers(ctx, api)
		if err != nil {
			return err
		}
		step("recently played containers before", "top", strings.Join(got[:min(5, len(got))], ", "), "container_rank", slices.Index(got, from.ID))
	}

	rep, err := activity.New(activity.Options{Client: api, Storefront: func() string { return sf.ID }, AppVersion: "0.1", Logger: log})
	if err != nil {
		return err
	}
	eng := native.New(native.Options{Client: api, CDMPath: cdm, Logger: log, IdleTimeout: -1})
	defer eng.Close()
	if err := eng.SetOutputDevice(ctx, outputDevice); err != nil {
		return err
	}
	coord, err := coordinator.New(coordinator.Options{Engine: eng, Logger: log, OnActivity: func(a coordinator.Activity) {
		step("activity", "kind", a.Kind, "item", a.Item.ID, "position", a.Position.Round(100*time.Millisecond), "reason", a.Reason)
		rep.Observe(a)
	}})
	if err != nil {
		return err
	}
	item := func(s applemusic.Resource[applemusic.SongAttributes]) coordinator.Item {
		return coordinator.Item{ID: s.ID, Title: s.Attributes.Name, Artist: s.Attributes.ArtistName, Duration: time.Duration(s.Attributes.DurationInMillis) * time.Millisecond, Container: from}
	}
	step("playing", "first", fmt.Sprintf("%s %q", first.ID, first.Attributes.Name), "second", fmt.Sprintf("%s %q", second.ID, second.Attributes.Name))
	if err := coord.SetQueue(ctx, []coordinator.Item{item(first), item(second)}, 0, "playspike"); err != nil {
		return err
	}
	listen = max(listen, 40*time.Second)
	time.Sleep(listen)
	if s := coord.Snapshot(); s.State != playback.StatePlaying {
		return fmt.Errorf("first song not playing after %v: %s %s", listen, s.State, s.Error)
	}
	if err := coord.Next(ctx); err != nil {
		return err
	}
	time.Sleep(5 * time.Second)
	if err := coord.Stop(ctx); err != nil {
		return err
	}
	if err := coord.Close(); err != nil {
		return err
	}
	cctx, ccancel := context.WithTimeout(ctx, 10*time.Second)
	defer ccancel()
	if err := rep.Close(cctx); err != nil {
		return err
	}
	step("play activity", "delivered", "yes (no errors logged above means Apple answered 2xx)")

	deadline := time.Now().Add(5 * time.Minute)
	songDone, containerDone := false, from.ID == ""
	for {
		if !songDone {
			after, err := recent(ctx, api)
			if err != nil {
				return err
			}
			i := slices.Index(after, first.ID)
			step("recently played", "top", strings.Join(after[:min(5, len(after))], ", "), "first_song_rank", i)
			if i >= 0 && i < slices.Index(append(before, first.ID), first.ID) || i == 0 {
				step("recently played: confirmed", "song", first.ID, "rank", i)
				songDone = true
			}
		}
		if !containerDone {
			got, err := recentContainers(ctx, api)
			if err != nil {
				return err
			}
			i := slices.Index(got, from.ID)
			step("recently played containers", "top", strings.Join(got[:min(5, len(got))], ", "), "container_rank", i)
			if i == 0 {
				step("recently played: container confirmed", "container", container)
				containerDone = true
			}
		}
		if songDone && containerDone {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("activity: Recently Played not updated within 5 minutes (song %v, container %v)", songDone, containerDone)
		}
		time.Sleep(20 * time.Second)
	}
}

// recentContainers lists the IDs of the user's recently played albums,
// playlists and stations ("Recently Played" in Apple Music), newest first.
func recentContainers(ctx context.Context, api *applemusic.Client) ([]string, error) {
	var page struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := api.Get(ctx, "/v1/me/recent/played", url.Values{"limit": {"10"}}, &page); err != nil {
		return nil, fmt.Errorf("recently played containers: %w", err)
	}
	ids := make([]string, len(page.Data))
	for i, d := range page.Data {
		ids[i] = d.ID
	}
	return ids, nil
}

// recent lists the IDs of the user's recently played tracks, newest first.
func recent(ctx context.Context, api *applemusic.Client) ([]string, error) {
	page, err := api.RecentlyPlayed(ctx, 30, 0)
	if err != nil {
		return nil, fmt.Errorf("recently played: %w", err)
	}
	ids := make([]string, 0, len(page.Data))
	for _, s := range page.Data {
		id := s.ID
		if c := s.Relationships.Catalog.Data; len(c) > 0 {
			id = c[0].ID
		}
		ids = append(ids, id)
	}
	return ids, nil
}
