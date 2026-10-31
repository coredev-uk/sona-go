package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/godbus/dbus/v5"
)

// xdg-desktop-portal names for the file chooser.
const (
	portalBusName                 = "org.freedesktop.portal.Desktop"
	portalPath    dbus.ObjectPath = "/org/freedesktop/portal/desktop"
	fileChooserIf                 = "org.freedesktop.portal.FileChooser"
	requestIf                     = "org.freedesktop.portal.Request"
	requestPrefix                 = "/org/freedesktop/portal/desktop/request/"
)

// Portal Request response codes.
const (
	responseSuccess   uint32 = 0
	responseCancelled uint32 = 1
)

// FileFilter is one entry of the chooser's file type list: a name and its
// glob patterns ("*.so").
type FileFilter struct {
	Name     string
	Patterns []string
}

// OpenFileOptions describe a FileChooser.OpenFile request.
type OpenFileOptions struct {
	Title       string
	AcceptLabel string
	// Filters are offered in order; the first is selected.
	Filters []FileFilter
	// Folder is the directory the dialog opens in (optional).
	Folder string
	// Parent identifies the parent window ("wayland:<handle>",
	// "x11:<xid>"); empty opens an unparented dialog.
	Parent string
}

// portalFilter is the portal's (sa(us)) filter: a name and (kind,
// pattern) pairs, kind 0 being a glob.
type portalFilter struct {
	Name     string
	Patterns []portalPattern
}

type portalPattern struct {
	Kind    uint32
	Pattern string
}

// SaveFileOptions describe a FileChooser.SaveFile request.
type SaveFileOptions struct {
	Title       string
	AcceptLabel string
	// Name is the suggested file name ("sona-settings.json").
	Name string
	// Filters are offered in order; the first is selected.
	Filters []FileFilter
	// Folder is the directory the dialog opens in (optional).
	Folder string
	// Parent identifies the parent window, as for OpenFileOptions.
	Parent string
}

// OpenFile asks xdg-desktop-portal's FileChooser for one local file and
// returns its path. It returns "" and no error when the user cancels.
// Cancelling ctx closes the dialog. conn is the session bus; it may be
// shared, OpenFile only adds and removes its own signal match.
func OpenFile(ctx context.Context, conn *dbus.Conn, opts OpenFileOptions) (string, error) {
	options := map[string]dbus.Variant{
		"multiple":  dbus.MakeVariant(false),
		"directory": dbus.MakeVariant(false),
	}
	chooserOptions(options, opts.AcceptLabel, opts.Filters, opts.Folder)
	return choose(ctx, conn, "OpenFile", opts.Parent, opts.Title, options)
}

// SaveFile asks xdg-desktop-portal's FileChooser where to save a file and
// returns the local path; the dialog confirms replacing an existing file.
// It returns "" and no error when the user cancels, and like OpenFile
// closes the dialog when ctx is cancelled. It does not create the file.
func SaveFile(ctx context.Context, conn *dbus.Conn, opts SaveFileOptions) (string, error) {
	options := map[string]dbus.Variant{}
	if opts.Name != "" {
		options["current_name"] = dbus.MakeVariant(opts.Name)
	}
	chooserOptions(options, opts.AcceptLabel, opts.Filters, opts.Folder)
	return choose(ctx, conn, "SaveFile", opts.Parent, opts.Title, options)
}

// Portal is the desktop's file chooser through xdg-desktop-portal on the
// session bus Conn (services.FileChooser on Linux).
type Portal struct{ Conn *dbus.Conn }

// OpenFile is OpenFile on p's bus.
func (p Portal) OpenFile(ctx context.Context, opts OpenFileOptions) (string, error) {
	return OpenFile(ctx, p.Conn, opts)
}

// SaveFile is SaveFile on p's bus.
func (p Portal) SaveFile(ctx context.Context, opts SaveFileOptions) (string, error) {
	return SaveFile(ctx, p.Conn, opts)
}

// chooserOptions adds the options OpenFile and SaveFile share.
func chooserOptions(options map[string]dbus.Variant, accept string, filters []FileFilter, folder string) {
	if accept != "" {
		options["accept_label"] = dbus.MakeVariant(accept)
	}
	if len(filters) > 0 {
		fs := make([]portalFilter, len(filters))
		for i, f := range filters {
			fs[i].Name = f.Name
			for _, p := range f.Patterns {
				fs[i].Patterns = append(fs[i].Patterns, portalPattern{Kind: 0, Pattern: p})
			}
		}
		options["filters"] = dbus.MakeVariant(fs)
		options["current_filter"] = dbus.MakeVariant(fs[0])
	}
	if folder != "" {
		// A NUL-terminated byte string, per the portal's "ay".
		options["current_folder"] = dbus.MakeVariant(append([]byte(folder), 0))
	}
}

// choose calls FileChooser.<method> as a modal request and waits for its
// Response: the chosen path, or "" when the user cancels.
func choose(ctx context.Context, conn *dbus.Conn, method, parent, title string, options map[string]dbus.Variant) (string, error) {
	names := conn.Names()
	if len(names) == 0 {
		return "", errors.New("app: file chooser: not connected")
	}
	token, err := handleToken()
	if err != nil {
		return "", err
	}
	// The portal derives the request path from our unique name and the
	// token; listening before the call means a fast reply is not missed.
	sender := strings.ReplaceAll(strings.TrimPrefix(names[0], ":"), ".", "_")
	want := dbus.ObjectPath(requestPrefix + sender + "/" + token)

	sigs := make(chan *dbus.Signal, 8)
	conn.Signal(sigs)
	defer conn.RemoveSignal(sigs)
	listen := func(path dbus.ObjectPath) (func(), error) {
		match := []dbus.MatchOption{
			dbus.WithMatchObjectPath(path),
			dbus.WithMatchInterface(requestIf),
			dbus.WithMatchMember("Response"),
		}
		if err := conn.AddMatchSignalContext(ctx, match...); err != nil {
			return nil, fmt.Errorf("app: file chooser: %w", err)
		}
		return func() { _ = conn.RemoveMatchSignal(match...) }, nil
	}
	stop, err := listen(want)
	if err != nil {
		return "", err
	}
	defer stop()

	options["handle_token"] = dbus.MakeVariant(token)
	options["modal"] = dbus.MakeVariant(true)

	var handle dbus.ObjectPath
	obj := conn.Object(portalBusName, portalPath)
	if err := obj.CallWithContext(ctx, fileChooserIf+"."+method, 0, parent, title, options).Store(&handle); err != nil {
		if ctx.Err() != nil {
			// The portal may have opened the dialog already: close it at
			// the path it would have used.
			_ = conn.Object(portalBusName, want).Call(requestIf+".Close", 0).Err
			return "", ctx.Err()
		}
		return "", fmt.Errorf("app: file chooser: %w", err)
	}
	if handle != want {
		// Portals older than 0.9 pick their own path.
		stop2, err := listen(handle)
		if err != nil {
			return "", err
		}
		defer stop2()
	}

	for {
		select {
		case <-ctx.Done():
			// Close the dialog; the context is already done.
			_ = conn.Object(portalBusName, handle).Call(requestIf+".Close", 0).Err
			return "", ctx.Err()
		case sig := <-sigs:
			if sig.Path != handle || sig.Name != requestIf+".Response" {
				continue
			}
			return chosenPath(sig.Body)
		}
	}
}

// chosenPath reads a Response(u response, a{sv} results) body.
func chosenPath(body []any) (string, error) {
	if len(body) != 2 {
		return "", fmt.Errorf("app: file chooser: malformed response (%d values)", len(body))
	}
	code, _ := body[0].(uint32)
	switch code {
	case responseSuccess:
	case responseCancelled:
		return "", nil
	default:
		return "", errors.New("app: file chooser: the dialog failed")
	}
	results, _ := body[1].(map[string]dbus.Variant)
	uris, _ := results["uris"].Value().([]string)
	if len(uris) == 0 {
		return "", nil
	}
	u, err := url.Parse(uris[0])
	if err != nil {
		return "", fmt.Errorf("app: file chooser: %w", err)
	}
	if u.Scheme != "file" || u.Path == "" {
		return "", fmt.Errorf("app: file chooser: not a local file: %s", uris[0])
	}
	return u.Path, nil
}

// handleToken is a fresh request token: letters, digits and underscores
// only, as the portal requires.
func handleToken() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("app: file chooser: %w", err)
	}
	return "sona_" + hex.EncodeToString(b), nil
}
