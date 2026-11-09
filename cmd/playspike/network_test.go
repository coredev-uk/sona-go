//go:build linux || (cgo && (darwin || windows))

package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"testing"

	"github.com/coredev-uk/sona-go/internal/applemusic"
)

func TestNetworkError(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"url error", fmt.Errorf("play: %w", &url.Error{Op: "Get", URL: "https://amp-api.music.apple.com/", Err: errors.New("no such host")}), true},
		{"timeout", fmt.Errorf("native: preview 1: %w", &net.OpError{Op: "read", Err: context.DeadlineExceeded}), true},
		{"unavailable", fmt.Errorf("native: preview 1: %w", &applemusic.StatusError{Code: 503}), true},
		{"throttled", &applemusic.StatusError{Code: 429}, true},
		{"refused", &applemusic.StatusError{Code: 403}, false},
		{"not found", &applemusic.StatusError{Code: 404}, false},
		{"no preview", fmt.Errorf("native: 1 has no preview: %w", applemusic.ErrNotPlayable), false},
		{"player", errors.New("preview did not advance: state error"), false},
		{"deadline", context.DeadlineExceeded, false},
		{"none", nil, false},
	} {
		if got := networkError(tc.err); got != tc.want {
			t.Errorf("%s: networkError(%v) = %v, want %v", tc.name, tc.err, got, tc.want)
		}
	}
}
