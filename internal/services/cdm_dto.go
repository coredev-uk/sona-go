package services

import (
	"github.com/coredev-uk/sona-go/internal/playback/cdm"
)

// EventCDMStatus carries CDMStatus on every change (latest wins).
const EventCDMStatus = "cdm:status"

// CDMStatus is the Widevine CDM the playback engine loads: Sona downloads
// it from Google after the first paint and keeps it up to date
// (internal/playback/cdm).
type CDMStatus struct {
	// State is "idle" (not downloaded yet: it starts after the first
	// paint), "downloading", "ready" or "failed"; "" when this platform's
	// engine needs no CDM.
	State string `json:"state"`
	// Version and Path are the CDM the next playback host loads.
	Version string `json:"version"`
	Path    string `json:"path"`
	// Code says why: when State is "failed", why there is no CDM; when
	// "ready", why the last update check failed ("" when it passed).
	// One of network, unsupported_platform, manifest_invalid,
	// untrusted_url, too_large, hash_mismatch, signature_invalid,
	// archive_invalid, load_failed, load_check_incomplete, disk.
	Code string `json:"code"`
	// Received and Total are the running download's progress in bytes.
	Received int64 `json:"received"`
	Total    int64 `json:"total"`
	// Updating: a newer version downloads while this one stays in use; it
	// applies to the next playback host.
	Updating bool `json:"updating"`
	// CheckedAt is the last completed update check, Unix ms (0: never).
	CheckedAt int64 `json:"checkedAt"`
}

func cdmStatus(s cdm.Status) CDMStatus {
	out := CDMStatus{
		State: string(s.State), Version: s.Version, Path: s.Path, Code: s.Code,
		Received: s.Received, Total: s.Total, Updating: s.Updating,
	}
	if !s.CheckedAt.IsZero() {
		out.CheckedAt = s.CheckedAt.UnixMilli()
	}
	return out
}
