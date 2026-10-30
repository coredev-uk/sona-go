package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/coredev-uk/sona-go/internal/app/dbustest"
)

// fakePortal is xdg-desktop-portal's FileChooser on a private bus. reply
// decides each request's Response; nil leaves the dialog open.
type fakePortal struct {
	conn   *dbus.Conn
	reply  func() (uint32, map[string]dbus.Variant)
	opts   chan map[string]dbus.Variant
	calls  chan string // the FileChooser method of each request
	closed chan dbus.ObjectPath
}

type fakeRequest struct {
	p    *fakePortal
	path dbus.ObjectPath
}

func (r fakeRequest) Close() *dbus.Error {
	r.p.closed <- r.path
	return nil
}

func (p *fakePortal) OpenFile(sender dbus.Sender, parent, title string, opts map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
	return p.request("OpenFile", sender, opts)
}

func (p *fakePortal) SaveFile(sender dbus.Sender, parent, title string, opts map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
	return p.request("SaveFile", sender, opts)
}

func (p *fakePortal) request(method string, sender dbus.Sender, opts map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
	p.calls <- method
	token, _ := opts["handle_token"].Value().(string)
	h := dbus.ObjectPath(requestPrefix + strings.ReplaceAll(strings.TrimPrefix(string(sender), ":"), ".", "_") + "/" + token)
	if err := p.conn.Export(fakeRequest{p, h}, h, requestIf); err != nil {
		return "", dbus.MakeFailedError(err)
	}
	p.opts <- opts
	if p.reply != nil {
		// Before the method returns: the client must already be listening.
		code, results := p.reply()
		if err := p.conn.Emit(h, requestIf+".Response", code, results); err != nil {
			return "", dbus.MakeFailedError(err)
		}
	}
	return h, nil
}

func startPortal(t *testing.T, reply func() (uint32, map[string]dbus.Variant)) (*fakePortal, *dbus.Conn) {
	t.Helper()
	addr := dbustest.Start(t)
	p := &fakePortal{
		conn:   dbustest.Conn(t, addr),
		reply:  reply,
		opts:   make(chan map[string]dbus.Variant, 1),
		calls:  make(chan string, 1),
		closed: make(chan dbus.ObjectPath, 1),
	}
	if err := p.conn.Export(p, portalPath, fileChooserIf); err != nil {
		t.Fatal(err)
	}
	if r, err := p.conn.RequestName(portalBusName, dbus.NameFlagDoNotQueue); err != nil || r != dbus.RequestNameReplyPrimaryOwner {
		t.Fatalf("own portal name: %v %v", r, err)
	}
	return p, dbustest.Conn(t, addr)
}

func TestOpenFileReturnsTheChosenPath(t *testing.T) {
	p, client := startPortal(t, func() (uint32, map[string]dbus.Variant) {
		return 0, map[string]dbus.Variant{"uris": dbus.MakeVariant([]string{"file:///opt/my%20chrome/libwidevinecdm.so"})}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	got, err := OpenFile(ctx, client, OpenFileOptions{
		Title:   "Choose",
		Filters: []FileFilter{{Name: "Shared libraries", Patterns: []string{"*.so"}}},
		Folder:  "/opt/google",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "/opt/my chrome/libwidevinecdm.so" {
		t.Fatalf("path = %q", got)
	}
	opts := <-p.opts
	if f, _ := opts["current_folder"].Value().([]byte); string(f) != "/opt/google\x00" {
		t.Fatalf("current_folder = %q", f)
	}
	if _, ok := opts["filters"]; !ok {
		t.Fatal("no filters sent")
	}
}

func TestOpenFileCancelledIsEmpty(t *testing.T) {
	_, client := startPortal(t, func() (uint32, map[string]dbus.Variant) {
		return 1, map[string]dbus.Variant{}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	got, err := OpenFile(ctx, client, OpenFileOptions{Title: "Choose"})
	if err != nil || got != "" {
		t.Fatalf("got %q, %v; want empty, nil", got, err)
	}
}

func TestOpenFileRejectsNonLocalURIs(t *testing.T) {
	_, client := startPortal(t, func() (uint32, map[string]dbus.Variant) {
		return 0, map[string]dbus.Variant{"uris": dbus.MakeVariant([]string{"sftp://host/lib.so"})}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := OpenFile(ctx, client, OpenFileOptions{Title: "Choose"}); err == nil {
		t.Fatal("want an error for a remote file")
	}
}

func TestOpenFileContextClosesTheDialog(t *testing.T) {
	p, client := startPortal(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := OpenFile(ctx, client, OpenFileOptions{Title: "Choose"})
		done <- err
	}()
	<-p.opts
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("OpenFile did not return")
	}
	select {
	case <-p.closed:
	case <-time.After(10 * time.Second):
		t.Fatal("Request.Close was not called")
	}
}

func TestSaveFileSuggestsANameAndReturnsThePath(t *testing.T) {
	p, client := startPortal(t, func() (uint32, map[string]dbus.Variant) {
		return 0, map[string]dbus.Variant{"uris": dbus.MakeVariant([]string{"file:///home/me/Backups/sona%20settings.json"})}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	got, err := SaveFile(ctx, client, SaveFileOptions{Title: "Export", Name: "sona-settings.json"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "/home/me/Backups/sona settings.json" {
		t.Fatalf("path = %q", got)
	}
	if m := <-p.calls; m != "SaveFile" {
		t.Fatalf("method = %s", m)
	}
	opts := <-p.opts
	if n, _ := opts["current_name"].Value().(string); n != "sona-settings.json" {
		t.Fatalf("current_name = %q", n)
	}
	if _, ok := opts["multiple"]; ok {
		t.Fatal("SaveFile sent OpenFile's multiple option")
	}
}
