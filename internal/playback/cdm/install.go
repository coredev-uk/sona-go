package cdm

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Options configures a Manager and Ensure. Dir is required; the rest
// default to Sona's production choices.
type Options struct {
	// Dir is the install directory (config.Paths.WidevineDir).
	Dir string
	// HTTP fetches the manifest and the package; nil uses a plain client.
	// Main passes one held by the first-paint gate.
	HTTP *http.Client
	// Platform is what to install; zero is the running one.
	Platform Platform
	// Source and Trust default to Mozilla and Widevine.
	Source Source
	Trust  Trust
	// LoadCheck, when set, loads a new library the way a playback host
	// does (Linux: a sandboxed probe child) before it becomes current; an
	// error rejects the version. Without it only the library's binary
	// format is checked.
	LoadCheck func(ctx context.Context, lib string) error
	// Override is SONA_WIDEVINE_CDM: a library, or the directory holding
	// one, used instead of any download (Manager only; Ensure ignores it).
	Override string
	Logger   *slog.Logger
	// Now and After are the clock (tests); defaults time.Now and
	// time.After.
	Now   func() time.Time
	After func(time.Duration) <-chan time.Time
	// UpdateInterval is the least time between update checks (default a
	// day); RetryInterval the wait after a failed check (default 30
	// minutes).
	UpdateInterval time.Duration
	RetryInterval  time.Duration
}

// EnvOverride names the developer override: a Widevine CDM library (or
// its directory) Sona loads instead of the downloaded one.
const EnvOverride = "SONA_WIDEVINE_CDM"

// Defaults for Options.
const (
	DefaultUpdateInterval = 24 * time.Hour
	DefaultRetryInterval  = 30 * time.Minute
	// checkTimeout bounds one update check, download included.
	checkTimeout = 30 * time.Minute
	// loadCheckTimeout bounds LoadCheck.
	loadCheckTimeout = time.Minute
)

// withDefaults fills in the defaults. Its error is a missing Dir, or the
// running platform having no CDM (CodeUnsupportedPlatform: everything else
// is still filled in).
func (o Options) withDefaults() (Options, error) {
	if o.Dir == "" {
		return o, errors.New("cdm: no install directory")
	}
	if o.HTTP == nil {
		o.HTTP = &http.Client{}
	}
	var perr error
	if o.Platform.GOOS == "" {
		o.Platform, perr = Current()
	}
	if o.Source.Manifests == nil {
		o.Source = Mozilla
	}
	if o.Trust == (Trust{}) {
		o.Trust = Widevine
	}
	if o.Logger == nil {
		o.Logger = slog.New(slog.DiscardHandler)
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.After == nil {
		o.After = time.After
	}
	if o.UpdateInterval <= 0 {
		o.UpdateInterval = DefaultUpdateInterval
	}
	if o.RetryInterval <= 0 {
		o.RetryInterval = DefaultRetryInterval
	}
	return o, perr
}

// Installed is a CDM ready to load.
type Installed struct {
	Version string
	Path    string // the library
}

// Ensure returns the installed CDM, downloading and installing the
// manifest's release first when there is none or the installed library
// is damaged (playspike, tests, CI). It reads the network only then.
func Ensure(ctx context.Context, o Options) (Installed, error) {
	o, err := o.withDefaults()
	if err != nil {
		return Installed{}, err
	}
	s := store{root: o.Dir, p: o.Platform}
	if err := os.MkdirAll(o.Dir, 0o700); err != nil {
		return Installed{}, &Error{Code: CodeDisk, Err: err}
	}
	if ptr, err := s.read(); err == nil && ptr.Current != "" && s.intact(ptr, ptr.Current) == nil {
		return Installed{Version: ptr.Current, Path: s.lib(ptr.Current)}, nil
	}
	ptr, _, err := check(ctx, o, nil)
	if err != nil {
		return Installed{}, err
	}
	return Installed{Version: ptr.Current, Path: s.lib(ptr.Current)}, nil
}

// FetchRelease reads the release Mozilla's manifest lists for o's
// platform (cmd/cdmfetch -version: CI's cache key).
func FetchRelease(ctx context.Context, o Options) (Release, error) {
	if o.Dir == "" {
		o.Dir = os.TempDir()
	}
	o, err := o.withDefaults()
	if err != nil {
		return Release{}, err
	}
	return fetchRelease(ctx, o.HTTP, o.Source, o.Platform)
}

// check runs one update check under the directory's lock: it reads the
// manifest and, when it lists a version newer than the current one (or
// nothing is installed), downloads, verifies and installs it and makes it
// current. It returns the pointer after the check and whether Current
// changed. A version that is older, current or rejected changes nothing.
func check(ctx context.Context, o Options, progress func(received, total int64)) (pointer, bool, error) {
	s := store{root: o.Dir, p: o.Platform}
	unlock, err := s.lock()
	if err != nil {
		return pointer{}, false, err
	}
	defer unlock()
	ptr, err := s.read()
	if err != nil {
		return ptr, false, err
	}
	if s.repair(&ptr, o.Logger) {
		if err := s.write(ptr); err != nil {
			return ptr, false, err
		}
	}

	rel, err := fetchRelease(ctx, o.HTTP, o.Source, o.Platform)
	if err != nil {
		return ptr, false, err
	}
	// CheckedAt moves only when the check completes: a failed one is
	// retried with the manager's backoff, not an update interval later.
	checked := o.Now()
	if ptr.Current != "" {
		switch {
		case compareVersions(rel.Version, ptr.Current) <= 0:
			// Current, or older: never downgrade (a stale or replayed
			// manifest can only point at older genuine packages).
			ptr.CheckedAt = checked
			return ptr, false, s.write(ptr)
		case ptr.Rejected[rel.Version] != "":
			o.Logger.Info("widevine: skipping a version that failed to load before", "version", rel.Version, "code", ptr.Rejected[rel.Version])
			ptr.CheckedAt = checked
			return ptr, false, s.write(ptr)
		}
	}

	t0 := time.Now()
	o.Logger.Info("widevine: downloading", "release", rel.String(), "platform", o.Platform.String())
	crx, err := download(ctx, o.HTTP, o.Source.PackageHosts, rel, o.Dir, progress)
	if err != nil {
		return ptr, false, err
	}
	defer func() { _ = os.Remove(crx) }()
	staging, err := os.MkdirTemp(o.Dir, stagingPrefix)
	if err != nil {
		return ptr, false, &Error{Code: CodeDisk, Err: err}
	}
	defer func() { _ = os.RemoveAll(staging) }()
	if err := unpack(crx, rel.Version, o, staging); err != nil {
		return ptr, false, err
	}
	o.Logger.Info("widevine: package verified", "version", rel.Version, "checks", "size, SHA-512, CRX3 signatures, extraction, library format")
	if err := s.place(staging, rel.Version); err != nil {
		return ptr, false, err
	}
	lib := s.lib(rel.Version)
	if o.LoadCheck != nil {
		lctx, cancel := context.WithTimeout(ctx, loadCheckTimeout)
		err := o.LoadCheck(lctx, lib)
		cancel()
		if err != nil {
			_ = os.RemoveAll(filepath.Join(o.Dir, rel.Version))
			if ptr.Current != "" {
				// Keep the working one; don't fetch this version again.
				if ptr.Rejected == nil {
					ptr.Rejected = map[string]string{}
				}
				ptr.Rejected[rel.Version] = CodeLoad
				_ = s.write(ptr)
			}
			return ptr, false, &Error{Code: CodeLoad, Err: err}
		}
		o.Logger.Info("widevine: load check passed", "version", rel.Version)
	}
	sum, err := s.hash(rel.Version)
	if err != nil {
		_ = os.RemoveAll(filepath.Join(o.Dir, rel.Version))
		return ptr, false, &Error{Code: CodeDisk, Err: err}
	}
	if ptr.Current != rel.Version {
		ptr.Previous = ptr.Current
	}
	ptr.Current, ptr.Failures, ptr.CheckedAt = rel.Version, 0, checked
	delete(ptr.Rejected, rel.Version)
	hashes := map[string]string{rel.Version: sum}
	if ptr.Previous != "" {
		hashes[ptr.Previous] = ptr.SHA256[ptr.Previous]
	}
	ptr.SHA256 = hashes
	if err := s.write(ptr); err != nil {
		return ptr, false, err
	}
	s.prune(ptr.Current, ptr.Previous)
	o.Logger.Info("widevine: installed", "version", rel.Version, "previous", ptr.Previous, "path", lib, "took", time.Since(t0).Round(time.Millisecond))
	return ptr, true, nil
}

// unpack verifies the CRX3 package at crx and extracts it into dir: the
// signatures against o.Trust, then the platform's files, whose component
// manifest must name version and whose library must be built for the
// platform.
func unpack(crx, version string, o Options, dir string) error {
	f, err := os.Open(crx)
	if err != nil {
		return &Error{Code: CodeDisk, Err: err}
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil {
		return &Error{Code: CodeDisk, Err: err}
	}
	a, err := verifyCRX3(f, st.Size(), o.Trust)
	if err != nil {
		return err
	}
	got, err := extract(f, a, o.Platform, dir)
	if err != nil {
		return err
	}
	if got != version {
		return fail(CodeArchive, "package is version %s, the manifest says %s", got, version)
	}
	return checkLibrary(filepath.Join(dir, o.Platform.Lib), o.Platform)
}
