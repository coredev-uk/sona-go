// Command releasesign writes update-<target>.json, the manifest Sona's
// self-updater reads (internal/update.Manifest, MyGo's format): the
// version, release notes, date, archive URL and size, the Ed25519 signature
// of the archive's SHA-256 digest (the one MyGo checks), and, for Sona, the
// digest in hex with a second signature over the statement
// `sona-update-v1 NUL target NUL version NUL channel NUL hex(sha256)`, which
// binds the version and platform to the archive (update.Statement).
//
//	SONA_UPDATE_KEY=… go run ./cmd/releasesign -target darwin-arm64 \
//	    -archive sona-0.3.0-darwin-arm64.tar.gz -version 0.3.0 \
//	    -url https://github.com/coredev-uk/sona-go/releases/download/v0.3.0/sona-0.3.0-darwin-arm64.tar.gz \
//	    -notes notes.md -out update-darwin-arm64.json
//
// The key is the base64 private key `go tool mygo keygen` writes, read from
// the environment variable named by -key-env (never a flag, which a process
// list would show). The publish job runs it last, so no install sees a
// manifest whose archive isn't uploaded yet.
//
// It signs nothing that the updater couldn't install: the archive must pass
// update.CheckArchive (MyGo's extraction rules and Sona's layout). The Windows
// archives of every release up to 0.5.0-rc.3 began with a "./" entry that the
// updater refuses, and no Windows install could update:
// `invalid path in update archive: "./"`.
// With -check it only does that check (no key, no manifest): package.sh runs it
// on the archive it has just made.
//
//	go run ./cmd/releasesign -check -target windows-amd64 -archive sona-0.3.0-windows-amd64.tar.gz
package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/coredev-uk/sona-go/internal/update"
)

func main() {
	if err := run(os.Args[1:], os.Getenv, time.Now); err != nil {
		fmt.Fprintln(os.Stderr, "releasesign:", err)
		os.Exit(1)
	}
}

func run(args []string, getenv func(string) string, now func() time.Time) error {
	fs := flag.NewFlagSet("releasesign", flag.ContinueOnError)
	target := fs.String("target", "", "manifest target: darwin-arm64 or windows-amd64")
	archive := fs.String("archive", "", "the update archive (gzip tar) to sign or check")
	check := fs.Bool("check", false, "only check that the updater can install -archive; sign nothing")
	version := fs.String("version", "", "the release version, without a leading v")
	url := fs.String("url", "", "where the archive is published (https)")
	notes := fs.String("notes", "", "file holding the release notes (Markdown), optional")
	date := fs.String("date", "", "build date, RFC 3339 (default: now)")
	out := fs.String("out", "", "manifest to write (default update-<target>.json)")
	keyEnv := fs.String("key-env", "SONA_UPDATE_KEY", "environment variable holding the base64 private key")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !validTarget(*target) {
		return fmt.Errorf("unknown -target %q", *target)
	}
	if *archive == "" {
		return errors.New("-archive is required")
	}
	if err := checkArchive(*archive, *target); err != nil {
		return err
	}
	if *check {
		return nil
	}
	if *url == "" || !update.Valid(*version) {
		return errors.New("-url and a semantic -version are required")
	}
	keyText := strings.TrimSpace(getenv(*keyEnv))
	if keyText == "" {
		return fmt.Errorf("$%s is empty: there is no key to sign with", *keyEnv)
	}
	raw, err := base64.StdEncoding.DecodeString(keyText)
	if err != nil || len(raw) != 64 {
		return fmt.Errorf("$%s is not a base64 Ed25519 private key", *keyEnv)
	}
	m := update.Manifest{Version: strings.TrimPrefix(*version, "v"), URL: *url, Date: *date}
	if m.Date == "" {
		m.Date = now().UTC().Format(time.RFC3339)
	}
	if *notes != "" {
		b, err := os.ReadFile(*notes)
		if err != nil {
			return err
		}
		m.Notes = strings.TrimSpace(string(b))
	}
	f, err := os.Open(*archive)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	sum := sha256.New()
	if m.Size, err = io.Copy(sum, f); err != nil {
		return err
	}
	digest := sum.Sum(nil)
	m.Signature = update.Sign(ed25519.PrivateKey(raw), digest)
	m.SHA256 = hex.EncodeToString(digest)
	m.StatementSignature = update.SignStatement(ed25519.PrivateKey(raw), *target, m.Version, digest)
	body, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	dest := *out
	if dest == "" {
		dest = update.ManifestName(*target)
	}
	return os.WriteFile(dest, append(body, '\n'), 0o644)
}

func validTarget(t string) bool { return t == "darwin-arm64" || t == "windows-amd64" }

// checkArchive refuses an archive the updater could not install.
func checkArchive(path, target string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if err := update.CheckArchive(f, target); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}
