package applemusic

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// PlayActivityURL is Apple's play-activity endpoint, the one MusicKit JS v3
// reports plays to (they feed Recently Played, history and royalties).
const PlayActivityURL = "https://universal-activity-service.itunes.apple.com/play"

// Play-activity event types ("event-type").
const (
	PlayEventEnd   = 0
	PlayEventStart = 1
)

// PlayEndReason is MusicKit's PlayActivityEndReasonType ("end-reason-type").
type PlayEndReason int

const (
	EndReasonNotApplicable        PlayEndReason = 0
	EndReasonOther                PlayEndReason = 1
	EndReasonSkippedForwards      PlayEndReason = 2
	EndReasonManuallyPaused       PlayEndReason = 3
	EndReasonSuspended            PlayEndReason = 4
	EndReasonSelectedOtherItem    PlayEndReason = 5 // MANUALLY_SELECTED_PLAYBACK_OF_A_DIFF_ITEM
	EndReasonPausedForInactivity  PlayEndReason = 6
	EndReasonNaturalEnd           PlayEndReason = 7
	EndReasonSessionTimeout       PlayEndReason = 8
	EndReasonBanned               PlayEndReason = 9
	EndReasonFailedToLoad         PlayEndReason = 10
	EndReasonPausedOnTimeout      PlayEndReason = 11
	EndReasonScrubBegin           PlayEndReason = 12
	EndReasonScrubEnd             PlayEndReason = 13
	EndReasonSkippedBackwards     PlayEndReason = 14
	EndReasonNotSupportedByClient PlayEndReason = 15
	EndReasonQuickPlay            PlayEndReason = 16
	EndReasonExitedApplication    PlayEndReason = 17
)

// Values MusicKit uses for a catalog or library song played by a MusicKit
// client with no container information.
const (
	PlayItemTypeStoreContent = 1  // "type": ITUNES_STORE_CONTENT
	PlayContainerUnknown     = 0  // "container-type": UNKNOWN
	PlayMediaTypeAudio       = 0  // "media-type": AUDIO
	PlaySourceMusicKit       = 10 // "source-type": MUSICKIT
	PlayFeatureMusicKit      = "music_kit-integration"
	playClientID             = "JSCLIENT"
	playEventType            = "JSPLAY"
)

// MusicKit's PlayActivityContainerType ("container-type") values.
const (
	PlayContainerRadio    = 1 // stations
	PlayContainerPlaylist = 2
	PlayContainerAlbum    = 3
	PlayContainerArtist   = 4
)

// MusicKit's "event-reason-hint-type" values for PLAY_START.
const (
	PlayHintNotSpecified     = 0 // no container, or the same as the previous play's
	PlayHintContainerChanged = 1
)

// PlayMode is MusicKit's "play-mode": the player's AutoPlay, repeat and
// shuffle state (MusicKit's AutoplayMode, RepeatPlayMode and
// ShufflePlayMode enums; 0 is unknown in each).
type PlayMode struct {
	AutoPlay int `json:"auto-play-mode"`    // 1 off, 2 on, 3 on but unsupported for the content
	Repeat   int `json:"repeat-play-mode"`  // 1 off, 2 one, 3 all
	Shuffle  int `json:"shuffle-play-mode"` // 1 off, 2 on
}

// PlayActivityEvent is one play-activity record in MusicKit's field names.
// It carries no credentials: SendPlayActivity adds the developer token and
// Music User Token, so events can be queued or persisted safely.
type PlayActivityEvent struct {
	EventType           int               `json:"event-type"`
	Type                int               `json:"type"`
	ContainerType       int               `json:"container-type"`
	ContainerIDs        map[string]string `json:"container-ids,omitempty"`
	IDs                 map[string]string `json:"ids,omitempty"`
	FeatureName         string            `json:"feature-name"`
	MediaType           int               `json:"media-type"`
	MediaDurationMS     int64             `json:"media-duration-in-milliseconds"`
	StartPositionMS     int64             `json:"start-position-in-milliseconds"`
	EndPositionMS       *int64            `json:"end-position-in-milliseconds,omitempty"`
	EndReasonType       *PlayEndReason    `json:"end-reason-type,omitempty"`
	EventReasonHintType *int              `json:"event-reason-hint-type,omitempty"`
	RecoData            string            `json:"reco-data,omitempty"`
	PlayMode            *PlayMode         `json:"play-mode,omitempty"`
	StoreFront          string            `json:"store-front"`
	Timestamp           int64             `json:"timestamp"` // ms since the epoch
	PersistentID        string            `json:"persistent-id"`
	BuildVersion        string            `json:"build-version"`
	UserAgent           string            `json:"user-agent"`
	UTCOffsetSeconds    int               `json:"utc-offset-in-seconds"`
	SourceType          int               `json:"source-type"`
	Offline             bool              `json:"offline"`
	PrivateEnabled      bool              `json:"private-enabled"`
	InternalBuild       bool              `json:"internal-build"`
	SiriInitiated       bool              `json:"siri-initiated"`
	Subscribed          bool              `json:"sb-enabled"`
}

// wireEvent is a PlayActivityEvent as sent.
type wireEvent struct {
	PlayActivityEvent
	DeveloperToken string `json:"developer-token"`
	UserToken      string `json:"user-token,omitempty"`
	SincePlayMS    int64  `json:"milliseconds-since-play"`
}

// SendPlayActivity posts events to PlayActivityURL as MusicKit does:
// {client_id:"JSCLIENT", event_type:"JSPLAY", data:[…]}, each event
// stamped with the developer token, the Music User Token and its age
// ("milliseconds-since-play"). The Music User Token also travels in the
// Media-User-Token header.
func (c *Client) SendPlayActivity(ctx context.Context, events []PlayActivityEvent) error {
	if len(events) == 0 {
		return errors.New("play activity: no events")
	}
	tok, err := c.Dev.Token(ctx)
	if err != nil {
		return fmt.Errorf("play activity: %w", err)
	}
	mut := ""
	if c.UserToken != nil {
		mut = c.UserToken()
	}
	if mut == "" {
		return fmt.Errorf("play activity: signed out: %w", ErrUnauthorized)
	}
	now := time.Now().UnixMilli()
	data := make([]wireEvent, len(events))
	for i, e := range events {
		data[i] = wireEvent{PlayActivityEvent: e, DeveloperToken: tok.JWT, UserToken: mut, SincePlayMS: max(now-e.Timestamp, 0)}
	}
	body := struct {
		ClientID  string      `json:"client_id"`
		EventType string      `json:"event_type"`
		Data      []wireEvent `json:"data"`
	}{playClientID, playEventType, data}
	if err := c.Do(ctx, Request{Method: http.MethodPost, URL: PlayActivityURL, Body: body}, nil); err != nil {
		return fmt.Errorf("play activity: %w", err)
	}
	return nil
}
