// Package discord sets the user's Discord rich presence over the local
// Discord IPC socket ($XDG_RUNTIME_DIR/discord-ipc-N, including the
// Flatpak and Snap locations, or $TMPDIR on macOS; see ipc_unix.go). On
// Windows, where Discord uses a named pipe, Supported is false. It needs no Discord credentials: only the
// application's public client ID.
//
// Wire format: every frame is a little-endian uint32 opcode, a
// little-endian uint32 payload length and a JSON payload. The client
// opens with a HANDSHAKE ({"v":1,"client_id":…}), the server answers with
// a READY dispatch, and each command frame carries a nonce its reply
// echoes.
package discord

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"sync"
	"time"
	"unicode/utf8"
)

// clientID is Sona's Discord application ID (public; it names the app in
// the presence and is not a secret). Create the application at
// https://discord.com/developers/applications (named "Sona", so the
// presence reads "Listening to Sona") and set its ID at build time:
//
//	go build -ldflags "-X github.com/coredev-uk/sona-go/internal/discord.clientID=<id>"
//
// (build/linux/Taskfile.yml passes SONA_DISCORD_CLIENT_ID). A build without
// it reads the same variable from the environment.
var clientID string

// ClientID returns the application ID built in (or from the environment);
// empty means presence is not configured.
func ClientID() string {
	if clientID != "" {
		return clientID
	}
	return os.Getenv("SONA_DISCORD_CLIENT_ID")
}

// Opcodes.
const (
	opHandshake uint32 = 0
	opFrame     uint32 = 1
	opClose     uint32 = 2
	opPing      uint32 = 3
	opPong      uint32 = 4
)

// maxFrame bounds a frame payload read from the socket.
const maxFrame = 1 << 20

// ActivityListening is Discord's "Listening to" activity type.
const ActivityListening = 2

// Activity is the presence shown while a track plays.
type Activity struct {
	Title  string
	Artist string
	Album  string
	// ArtworkURL is a public https image URL (Discord fetches it); empty
	// shows the application's icon.
	ArtworkURL string
	Start, End time.Time // the track's start and end on the wall clock
}

// ErrUnsupported is returned where Sona has no transport to the Discord
// app (Supported is false).
var ErrUnsupported = fmt.Errorf("discord: presence is not available on this platform: %w", errors.ErrUnsupported)

// ErrNotConfigured is returned when the client ID is empty.
var ErrNotConfigured = errors.New("discord: no application client ID in this build")

// Client is one Discord IPC connection, dialled on demand. Its methods are
// safe for concurrent use.
type Client struct {
	clientID string
	paths    func() []string

	mu    sync.Mutex
	conn  net.Conn
	nonce uint64
}

// New returns a Client for the application clientID that looks for the
// socket in the standard places.
func New(clientID string) *Client {
	return &Client{clientID: clientID, paths: socketPaths}
}

// Connected reports whether a connection is open (it may since have been
// closed by Discord; the next call notices).
func (c *Client) Connected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn != nil
}

// SetActivity shows a (or clears the presence when a is nil). It connects
// first if needed, and redials once when the connection turns out to be
// stale.
func (c *Client) SetActivity(ctx context.Context, a *Activity) error {
	if c.clientID == "" {
		return ErrNotConfigured
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	fresh := c.conn == nil
	if fresh {
		if err := c.connect(ctx); err != nil {
			return err
		}
	}
	err := c.setActivity(ctx, a)
	if err == nil || fresh || !isConnErr(err) {
		return err
	}
	// The connection was stale (Discord restarted): redial once.
	if err := c.connect(ctx); err != nil {
		return err
	}
	return c.setActivity(ctx, a)
}

// Close closes the connection; the next SetActivity reconnects.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.drop()
}

func (c *Client) drop() error {
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

// connErr marks transport failures, which a redial may cure.
type connErr struct{ err error }

func (e connErr) Error() string { return e.err.Error() }
func (e connErr) Unwrap() error { return e.err }

func isConnErr(err error) bool {
	var ce connErr
	return errors.As(err, &ce)
}

// connect dials the first socket that answers and performs the handshake.
func (c *Client) connect(ctx context.Context) error {
	_ = c.drop()
	if !Supported {
		return ErrUnsupported
	}
	var d net.Dialer
	var conn net.Conn
	for _, p := range c.paths() {
		cn, err := d.DialContext(ctx, "unix", p)
		if err == nil {
			conn = cn
			break
		}
	}
	if conn == nil {
		return errors.New("discord: not running (no IPC socket)")
	}
	c.conn = conn
	hello, _ := json.Marshal(map[string]any{"v": 1, "client_id": c.clientID})
	if err := c.write(ctx, opHandshake, hello); err != nil {
		_ = c.drop()
		return err
	}
	for {
		op, data, err := c.read(ctx)
		if err != nil {
			_ = c.drop()
			return err
		}
		switch op {
		case opClose:
			_ = c.drop()
			return fmt.Errorf("discord: handshake refused: %s", closeReason(data))
		case opFrame:
			var m message
			if err := json.Unmarshal(data, &m); err != nil {
				_ = c.drop()
				return fmt.Errorf("discord: handshake reply: %w", err)
			}
			if m.Cmd == "DISPATCH" && m.Evt == "READY" {
				return nil
			}
			if m.Evt == "ERROR" {
				_ = c.drop()
				return fmt.Errorf("discord: handshake: %s", m.errMessage())
			}
		}
	}
}

// message is the envelope of a Discord IPC frame payload.
type message struct {
	Cmd   string          `json:"cmd"`
	Evt   string          `json:"evt,omitempty"`
	Nonce string          `json:"nonce,omitempty"`
	Args  any             `json:"args,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
}

func (m message) errMessage() string {
	var e struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if json.Unmarshal(m.Data, &e) == nil && e.Message != "" {
		return fmt.Sprintf("%s (%d)", e.Message, e.Code)
	}
	return string(m.Data)
}

func closeReason(data []byte) string {
	var e struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if json.Unmarshal(data, &e) == nil && e.Message != "" {
		return fmt.Sprintf("%s (%d)", e.Message, e.Code)
	}
	return string(data)
}

// setActivity sends SET_ACTIVITY and waits for its reply.
func (c *Client) setActivity(ctx context.Context, a *Activity) error {
	c.nonce++
	nonce := strconv.FormatUint(c.nonce, 10)
	args := map[string]any{"pid": os.Getpid()}
	if a != nil {
		args["activity"] = activityPayload(a)
	}
	payload, err := json.Marshal(message{Cmd: "SET_ACTIVITY", Nonce: nonce, Args: args})
	if err != nil {
		return fmt.Errorf("discord: %w", err)
	}
	if err := c.write(ctx, opFrame, payload); err != nil {
		_ = c.drop()
		return err
	}
	for {
		op, data, err := c.read(ctx)
		if err != nil {
			_ = c.drop()
			return err
		}
		switch op {
		case opPing:
			if err := c.write(ctx, opPong, data); err != nil {
				_ = c.drop()
				return err
			}
		case opClose:
			_ = c.drop()
			return connErr{fmt.Errorf("discord: closed: %s", closeReason(data))}
		case opFrame:
			var m message
			if err := json.Unmarshal(data, &m); err != nil || m.Nonce != nonce {
				continue
			}
			if m.Evt == "ERROR" {
				return fmt.Errorf("discord: SET_ACTIVITY: %s", m.errMessage())
			}
			return nil
		}
	}
}

// activityPayload is the SET_ACTIVITY activity object.
func activityPayload(a *Activity) map[string]any {
	act := map[string]any{
		"type":    ActivityListening,
		"details": field(a.Title),
		"state":   field(a.Artist),
	}
	ts := map[string]int64{}
	if !a.Start.IsZero() {
		ts["start"] = a.Start.UnixMilli()
	}
	if !a.End.IsZero() {
		ts["end"] = a.End.UnixMilli()
	}
	if len(ts) > 0 {
		act["timestamps"] = ts
	}
	assets := map[string]string{}
	if a.ArtworkURL != "" {
		assets["large_image"] = a.ArtworkURL
	}
	if a.Album != "" {
		assets["large_text"] = field(a.Album)
	}
	if len(assets) > 0 {
		act["assets"] = assets
	}
	return act
}

// field fits s to Discord's activity string limits: 2 to 128 characters.
// Discord counts UTF-16 units; truncating at 127 runes stays inside the
// limit for text outside the astral planes, and a one-character value is
// padded with a zero-width space.
func field(s string) string {
	if utf8.RuneCountInString(s) > 128 {
		r := []rune(s)
		s = string(r[:127]) + "…"
	}
	if utf8.RuneCountInString(s) < 2 {
		s += "\u200b"
		if s == "\u200b" {
			s += "\u200b"
		}
	}
	return s
}

// write sends one frame, bounded by ctx's deadline.
func (c *Client) write(ctx context.Context, op uint32, payload []byte) error {
	setDeadline(ctx, c.conn)
	buf := make([]byte, 8+len(payload))
	binary.LittleEndian.PutUint32(buf[0:], op)
	binary.LittleEndian.PutUint32(buf[4:], uint32(len(payload)))
	copy(buf[8:], payload)
	if _, err := c.conn.Write(buf); err != nil {
		return connErr{fmt.Errorf("discord: write: %w", err)}
	}
	return nil
}

// read receives one frame, bounded by ctx's deadline.
func (c *Client) read(ctx context.Context) (uint32, []byte, error) {
	setDeadline(ctx, c.conn)
	var hdr [8]byte
	if _, err := io.ReadFull(c.conn, hdr[:]); err != nil {
		return 0, nil, connErr{fmt.Errorf("discord: read: %w", err)}
	}
	op := binary.LittleEndian.Uint32(hdr[0:])
	n := binary.LittleEndian.Uint32(hdr[4:])
	if n > maxFrame {
		return 0, nil, fmt.Errorf("discord: frame of %d bytes", n)
	}
	data := make([]byte, n)
	if _, err := io.ReadFull(c.conn, data); err != nil {
		return 0, nil, connErr{fmt.Errorf("discord: read: %w", err)}
	}
	return op, data, nil
}

func setDeadline(ctx context.Context, conn net.Conn) {
	dl, _ := ctx.Deadline()
	_ = conn.SetDeadline(dl) // zero: none
}
