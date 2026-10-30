package applemusic

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestSendPlayActivity(t *testing.T) {
	c, seen := playClient(t, 202, `{}`)
	reason := EndReasonNaturalEnd
	end := int64(318187)
	ts := time.Now().Add(-2 * time.Second).UnixMilli()
	err := c.SendPlayActivity(context.Background(), []PlayActivityEvent{{
		EventType: PlayEventEnd, Type: PlayItemTypeStoreContent, IDs: map[string]string{"subscription-adam-id": "1109715168"},
		FeatureName: PlayFeatureMusicKit, MediaDurationMS: 318187, StartPositionMS: 90000, EndPositionMS: &end,
		EndReasonType: &reason, StoreFront: "gb", Timestamp: ts, PersistentID: "0123456789abcdef", SourceType: PlaySourceMusicKit, Subscribed: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	r := (*seen)[0]
	if r.path != "/play" || r.header.Get("Media-User-Token") != "mut" || r.header.Get("Authorization") != "Bearer dev-0" {
		t.Fatalf("request %s %v", r.path, r.header)
	}
	if r.body["client_id"] != "JSCLIENT" || r.body["event_type"] != "JSPLAY" {
		t.Fatalf("envelope %v", r.body)
	}
	data := r.body["data"].([]any)
	ev := data[0].(map[string]any)
	want := map[string]any{
		"event-type": 0.0, "type": 1.0, "container-type": 0.0, "feature-name": "music_kit-integration", "media-type": 0.0,
		"media-duration-in-milliseconds": 318187.0, "start-position-in-milliseconds": 90000.0, "end-position-in-milliseconds": 318187.0,
		"end-reason-type": 7.0, "store-front": "gb", "timestamp": float64(ts), "persistent-id": "0123456789abcdef",
		"source-type": 10.0, "sb-enabled": true, "offline": false, "private-enabled": false, "siri-initiated": false,
		"internal-build": false, "developer-token": "dev-0", "user-token": "mut",
	}
	for k, v := range want {
		if ev[k] != v {
			t.Errorf("%s = %v, want %v", k, ev[k], v)
		}
	}
	if ids := ev["ids"].(map[string]any); ids["subscription-adam-id"] != "1109715168" {
		t.Errorf("ids %v", ids)
	}
	if ms := ev["milliseconds-since-play"].(float64); ms < 2000 || ms > 10000 {
		t.Errorf("milliseconds-since-play %v", ms)
	}
	for _, k := range []string{"event-reason-hint-type", "container-ids", "reco-data"} {
		if _, ok := ev[k]; ok {
			t.Errorf("unset %s sent", k)
		}
	}
}

func TestPlayActivityEventCarriesNoCredentials(t *testing.T) {
	b, err := json.Marshal(PlayActivityEvent{EventType: PlayEventStart})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	for _, k := range []string{"developer-token", "user-token"} {
		if _, ok := m[k]; ok {
			t.Fatalf("%s in a stored event", k)
		}
	}
}

func TestSendPlayActivityErrors(t *testing.T) {
	c, _ := playClient(t, http.StatusForbidden, `{}`)
	if err := c.SendPlayActivity(context.Background(), []PlayActivityEvent{{}}); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("403: %v", err)
	}
	c, _ = playClient(t, 200, `{}`)
	c.UserToken = func() string { return "" }
	if err := c.SendPlayActivity(context.Background(), []PlayActivityEvent{{}}); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("signed out: %v", err)
	}
	if err := c.SendPlayActivity(context.Background(), nil); err == nil {
		t.Fatal("empty batch accepted")
	}
}
