package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// desktopFile is Sona's desktop entry as the MIME applications database
// names it.
const desktopFile = appID + ".desktop"

// XDGLinkHandler is the XDG default application for the Apple Music link
// schemes (LinkSchemes), read and written with xdg-mime, which edits the
// user's mimeapps.list: services.LinkHandler on Linux.
//
// Only an installed Sona has a desktop entry for the default to name. Without
// one (an AppImage that isn't integrated, a source build) or inside a Flatpak
// sandbox (whose xdg-mime can't reach the host's list) both methods fail with
// errors.ErrUnsupported.
type XDGLinkHandler struct{}

// xdgMime runs xdg-mime and returns its standard output.
func xdgMime(ctx context.Context, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, "xdg-mime", args...).Output()
	return string(out), err
}

// linkHandlerUsable reports why Sona can't be chosen here, nil when it can.
func linkHandlerUsable() error {
	if _, err := os.Stat("/.flatpak-info"); err == nil {
		return fmt.Errorf("app: link handler: inside a Flatpak sandbox: %w", errors.ErrUnsupported)
	}
	if _, err := exec.LookPath("xdg-mime"); err != nil {
		return fmt.Errorf("app: link handler: no xdg-mime: %w", errors.ErrUnsupported)
	}
	if !desktopFileInstalled() {
		return fmt.Errorf("app: link handler: %s isn't installed: %w", desktopFile, errors.ErrUnsupported)
	}
	return nil
}

// desktopFileInstalled reports whether Sona's desktop entry is in one of the
// applications directories of the XDG base directory spec.
func desktopFileInstalled() bool {
	dirs := []string{}
	if d := os.Getenv("XDG_DATA_HOME"); d != "" && filepath.IsAbs(d) {
		dirs = append(dirs, d)
	} else if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".local", "share"))
	}
	data := os.Getenv("XDG_DATA_DIRS")
	if data == "" {
		data = "/usr/local/share:/usr/share"
	}
	dirs = append(dirs, filepath.SplitList(data)...)
	for _, d := range dirs {
		if _, err := os.Stat(filepath.Join(d, "applications", desktopFile)); err == nil {
			return true
		}
	}
	return false
}

// xdgMimeTimeout bounds the queries: xdg-mime reads a few small files.
const xdgMimeTimeout = 10 * time.Second

func mimeType(scheme string) string { return "x-scheme-handler/" + scheme }

// Default reports whether Sona's desktop entry is the default for every link
// scheme and, when it isn't, the desktop entry that is for the first scheme
// it doesn't have (without its .desktop suffix; "" when none is set).
func (XDGLinkHandler) Default() (sona bool, other string, err error) {
	if err := linkHandlerUsable(); err != nil {
		return false, "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), xdgMimeTimeout)
	defer cancel()
	sona = true
	for _, scheme := range LinkSchemes {
		out, err := xdgMime(ctx, "query", "default", mimeType(scheme))
		if err != nil {
			return false, "", fmt.Errorf("app: link handler: xdg-mime query: %w", err)
		}
		if got := strings.TrimSpace(out); got != desktopFile {
			if sona {
				other = strings.TrimSuffix(got, ".desktop")
			}
			sona = false
		}
	}
	return sona, other, nil
}

// MakeDefault makes Sona's desktop entry the default for every link scheme.
func (XDGLinkHandler) MakeDefault(ctx context.Context) error {
	if err := linkHandlerUsable(); err != nil {
		return err
	}
	for _, scheme := range LinkSchemes {
		if _, err := xdgMime(ctx, "default", desktopFile, mimeType(scheme)); err != nil {
			return fmt.Errorf("app: link handler: xdg-mime default %s: %w", mimeType(scheme), err)
		}
	}
	return nil
}
