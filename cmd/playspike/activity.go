package main

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/activity"
	"github.com/coredev-uk/sona-go/internal/playback/coordinator"
	"github.com/coredev-uk/sona-go/internal/playback/native"
)

// runActivity proves play-activity reporting: it plays term's song for
// listen (≥30 s), skips to nextTerm's song, stops, and polls Apple's
// Recently Played until the first song shows up at the top.
func runActivity(log *slog.Logger, envFile, term, nextTerm string, listen time.Duration, cdm string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	api, sf, err := signIn(ctx, envFile)
	if err != nil {
		return err
	}
	first, err := resolve(ctx, api, sf.ID, "", term)
	if err != nil {
		return err
	}
	second, err := resolve(ctx, api, sf.ID, "", nextTerm)
	if err != nil {
		return err
	}
	before, err := recent(ctx, api)
	if err != nil {
		return err
	}
	step("recently played before", "top", strings.Join(before[:min(5, len(before))], ", "))

	rep, err := activity.New(activity.Options{Client: api, Storefront: func() string { return sf.ID }, AppVersion: "0.1", Logger: log})
	if err != nil {
		return err
	}
	eng := native.New(native.Options{Client: api, CDMPath: cdm, Logger: log, IdleTimeout: -1})
	defer eng.Close()
	coord, err := coordinator.New(coordinator.Options{Engine: eng, Logger: log, OnActivity: func(a coordinator.Activity) {
		step("activity", "kind", a.Kind, "item", a.Item.ID, "position", a.Position.Round(100*time.Millisecond), "reason", a.Reason)
		rep.Observe(a)
	}})
	if err != nil {
		return err
	}
	item := func(s applemusic.Resource[applemusic.SongAttributes]) coordinator.Item {
		return coordinator.Item{ID: s.ID, Title: s.Attributes.Name, Artist: s.Attributes.ArtistName, Duration: time.Duration(s.Attributes.DurationInMillis) * time.Millisecond}
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
	for {
		after, err := recent(ctx, api)
		if err != nil {
			return err
		}
		i := slices.Index(after, first.ID)
		step("recently played", "top", strings.Join(after[:min(5, len(after))], ", "), "first_song_rank", i)
		if i >= 0 && i < slices.Index(append(before, first.ID), first.ID) || i == 0 {
			step("recently played: confirmed", "song", first.ID, "rank", i)
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("song %s did not move up Recently Played within 5 minutes", first.ID)
		}
		time.Sleep(20 * time.Second)
	}
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
