package update

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// RateLimitError is GitHub refusing a request because the network's
// allowance is used up: anonymous callers share 60 API calls an hour per IP
// address, with everything else on the network. A check can't succeed before
// Reset, so nobody should ask again until then.
type RateLimitError struct {
	// Reset is when GitHub says the allowance returns: the later of
	// x-ratelimit-reset and now plus Retry-After. An hour from the answer
	// when it names neither.
	Reset time.Time
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("update: GitHub's rate limit is reached until %s", e.Reset.UTC().Format(time.RFC3339))
}

// rateLimit reads resp as GitHub's limit: a 403 or 429 whose
// x-ratelimit-remaining is 0, or that carries Retry-After (the secondary
// limits). now is when the answer arrived. Any other 403, such as a
// forbidden resource, is not one.
func rateLimit(resp *http.Response, now time.Time) (*RateLimitError, bool) {
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusTooManyRequests {
		return nil, false
	}
	retry := strings.TrimSpace(resp.Header.Get("Retry-After"))
	if retry == "" && strings.TrimSpace(resp.Header.Get("X-RateLimit-Remaining")) != "0" {
		return nil, false
	}
	var reset time.Time
	if secs, err := strconv.ParseInt(strings.TrimSpace(resp.Header.Get("X-RateLimit-Reset")), 10, 64); err == nil && secs > 0 {
		reset = time.Unix(secs, 0)
	}
	// Retry-After is a number of seconds or an HTTP date.
	if secs, err := strconv.ParseInt(retry, 10, 32); err == nil && secs >= 0 {
		if t := now.Add(time.Duration(secs) * time.Second); t.After(reset) {
			reset = t
		}
	} else if t, err := http.ParseTime(retry); err == nil && t.After(reset) {
		reset = t
	}
	if reset.IsZero() {
		reset = now.Add(time.Hour)
	}
	return &RateLimitError{Reset: reset}, true
}
