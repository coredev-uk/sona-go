// Package own is the add-on side of Sona's own add-ons (Last.fm,
// ListenBrainz and Discord; #302): the client of protocol v2 that
// `sona --addon <id>` runs, speaking over its standard input and output to
// the supervisor like any other add-on (docs/addons.md).
//
// The code is the signed Sona executable itself. It gets no more of the
// channel than a third-party add-on does, apart from what docs/addons.md
// lists for bundled add-ons (Sona's requests of it, and the build
// credentials in hello's answer).
package own

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/coredev-uk/sona-go/internal/cli"
)

// maxFrame is the longest frame the client reads: the session's limit
// (cli.AddonMaxFrame).
const maxFrame = cli.AddonMaxFrame

// requestTimeout bounds each request of Sona the client handles, and the
// time it waits for Sona's answer to its own: a call that outlives the
// session's deadline is of no use to anyone.
const requestTimeout = 30 * time.Second

// Handler answers one request of Sona. params is the request's params; the
// result is encoded as JSON. An *cli.Error keeps its code; any other error
// is the code "failed" (its text goes to the add-on's log, never the
// protocol).
type Handler func(ctx context.Context, params json.RawMessage) (any, error)

// Client is one add-on's end of the channel. Handle and OnEvent are set
// before Serve.
type Client struct {
	id  string
	log *slog.Logger
	in  io.Reader

	wmu sync.Mutex // one frame at a time
	out io.Writer

	mu       sync.Mutex
	next     int64
	pending  map[int64]chan reply
	handlers map[string]Handler
	events   map[string]func(json.RawMessage)
	hello    cli.HelloResult
	stopped  bool
	stop     chan struct{} // closed when Sona sent shutdown
	ready    chan struct{} // closed once hello is answered and Setup has run
}

type reply struct {
	result json.RawMessage
	err    *cli.Error
}

// NewClient returns the client of add-on id over in and out, logging to log
// (the add-on's standard error: Sona's log of it).
func NewClient(id string, in io.Reader, out io.Writer, log *slog.Logger) *Client {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Client{
		id: id, log: log, in: in, out: out,
		pending: map[int64]chan reply{}, handlers: map[string]Handler{}, events: map[string]func(json.RawMessage){},
		stop: make(chan struct{}), ready: make(chan struct{}),
	}
}

// Handle sets the answer to Sona's request method; the one request Sona
// sends every add-on, invoke, is answered "unknown_method" unless set.
func (c *Client) Handle(method string, h Handler) {
	c.mu.Lock()
	c.handlers[method] = h
	c.mu.Unlock()
}

// OnEvent sets what runs for each event of topic (after Subscribe). fn runs
// on the reading goroutine: it must not block.
func (c *Client) OnEvent(topic string, fn func(data json.RawMessage)) {
	c.mu.Lock()
	c.events[topic] = fn
	c.mu.Unlock()
}

// Hello is what Sona answered hello with; valid after Serve has said hello.
func (c *Client) Hello() cli.HelloResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hello
}

// Setup runs once hello is answered, with Sona's answer, before the client
// starts to wait: subscribe to topics, read a secret. Its error ends the
// add-on.
type Setup func(ctx context.Context, c *Client, hello cli.HelloResult) error

// Serve says hello with the permissions asked, runs setup, then answers
// Sona's requests and delivers its events until Sona stops the add-on (the
// shutdown event, or the end of the channel) or ctx ends. A clean stop
// returns nil.
func (c *Client) Serve(ctx context.Context, permissions []string, setup Setup) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	readDone := make(chan error, 1)
	go func() {
		err := c.read(ctx)
		c.failPending()
		readDone <- err
	}()

	var res cli.HelloResult
	hello := cli.HelloParams{
		ID: c.id, Permissions: append([]string{}, permissions...),
		Protocol: cli.VersionRange{Min: cli.AddonProtocolMin, Max: cli.AddonProtocolMax},
	}
	if err := c.Call(ctx, cli.AddonMethodHello, hello, &res); err != nil {
		return fmt.Errorf("hello: %w", err)
	}
	c.mu.Lock()
	c.hello = res
	c.mu.Unlock()
	if setup != nil {
		if err := setup(ctx, c, res); err != nil {
			return err
		}
	}
	close(c.ready)
	select {
	case err := <-readDone:
		return err
	case <-c.stop:
		return nil
	case <-ctx.Done():
		return nil
	}
}

// Call sends a request to Sona and waits for its answer, decoding the
// result into out (nil: ignored). A refusal is an *cli.Error with its code.
func (c *Client) Call(ctx context.Context, method string, params, out any) error {
	raw, err := json.Marshal(params)
	if err != nil {
		return err
	}
	ch := make(chan reply, 1)
	c.mu.Lock()
	c.next++
	id := c.next
	c.pending[id] = ch
	c.mu.Unlock()
	forget := func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}
	if err := c.send(cli.Frame{ID: id, Method: method, Params: raw}); err != nil {
		forget()
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	select {
	case r := <-ch:
		if r.err != nil {
			return r.err
		}
		if out != nil && len(r.result) > 0 {
			return json.Unmarshal(r.result, out)
		}
		return nil
	case <-ctx.Done():
		forget()
		return ctx.Err()
	}
}

// Subscribe subscribes to topics.
func (c *Client) Subscribe(ctx context.Context, topics ...string) error {
	return c.Call(ctx, cli.AddonMethodSubscribe, cli.TopicsParams{Topics: topics}, nil)
}

// Secret reads one of the add-on's own secrets. found is false while there
// is none.
func (c *Client) Secret(ctx context.Context, key string) (value string, found bool, err error) {
	var r cli.SecretResult
	err = c.Call(ctx, cli.AddonMethodSecret, cli.SecretParams{Key: key}, &r)
	var ce *cli.Error
	if errors.As(err, &ce) && ce.Code == "not_found" {
		return "", false, nil
	}
	return r.Value, err == nil, err
}

func (c *Client) send(f cli.Frame) error {
	b, err := json.Marshal(f)
	if err != nil {
		return err
	}
	c.wmu.Lock()
	defer c.wmu.Unlock()
	_, err = c.out.Write(append(b, '\n'))
	return err
}

// read handles Sona's frames until the channel ends: answers to our
// requests, Sona's requests, events.
func (c *Client) read(ctx context.Context) error {
	br := bufio.NewReaderSize(c.in, 64<<10)
	var wg sync.WaitGroup
	defer wg.Wait()
	for {
		line, err := readLine(br)
		if err != nil {
			if errors.Is(err, io.EOF) || ctx.Err() != nil {
				return nil
			}
			return err
		}
		if len(line) == 0 {
			continue
		}
		var f cli.Frame
		if err := json.Unmarshal(line, &f); err != nil {
			c.log.Warn("sona sent a frame that isn't JSON")
			continue
		}
		switch {
		case f.Event != "":
			if f.Event == cli.EventShutdown {
				c.markStopped()
				return nil
			}
			c.mu.Lock()
			fn := c.events[f.Event]
			c.mu.Unlock()
			if fn != nil {
				fn(f.Data)
			}
		case f.Method != "" && f.ID > 0:
			wg.Add(1)
			go func() {
				defer wg.Done()
				c.serveRequest(ctx, f)
			}()
		case f.ID > 0:
			c.mu.Lock()
			ch := c.pending[f.ID]
			delete(c.pending, f.ID)
			c.mu.Unlock()
			if ch != nil {
				ch <- reply{result: f.Result, err: f.Error}
			}
		}
	}
}

// failPending answers every request still waiting: the channel is gone.
func (c *Client) failPending() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, ch := range c.pending {
		delete(c.pending, id)
		ch <- reply{err: &cli.Error{Code: cli.AddonErrNotRunning, Message: "the channel to Sona ended"}}
	}
}

func (c *Client) markStopped() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.stopped {
		c.stopped = true
		close(c.stop)
	}
}

// serveRequest answers one request of Sona.
func (c *Client) serveRequest(ctx context.Context, f cli.Frame) {
	// Sona may send a request the moment it has accepted hello: it waits for
	// the answer to be read and Setup to have run, which is what its handlers
	// may need (the hello's credentials).
	select {
	case <-c.ready:
	case <-ctx.Done():
		return
	}
	c.mu.Lock()
	h := c.handlers[f.Method]
	c.mu.Unlock()
	out := cli.Frame{ID: f.ID}
	if h == nil {
		out.Error = &cli.Error{Code: cli.AddonErrUnknownMethod, Message: "no such method"}
	} else {
		ctx, cancel := context.WithTimeout(ctx, requestTimeout)
		res, err := h(ctx, f.Params)
		cancel()
		switch {
		case err != nil:
			var ce *cli.Error
			if !errors.As(err, &ce) {
				c.log.Warn("request failed", "method", f.Method, "err", err)
				ce = &cli.Error{Code: cli.AddonErrFailed}
			}
			out.Error = &cli.Error{Code: ce.Code}
		default:
			b, merr := json.Marshal(res)
			if merr != nil {
				out.Error = &cli.Error{Code: cli.AddonErrFailed}
			} else {
				out.Result = b
			}
		}
	}
	if err := c.send(out); err != nil {
		c.log.Warn("couldn't answer Sona", "method", f.Method)
	}
}

// readLine reads one frame: a line of at most maxFrame bytes.
func readLine(r *bufio.Reader) ([]byte, error) {
	var line []byte
	for {
		part, err := r.ReadSlice('\n')
		line = append(line, part...)
		if len(line) > maxFrame+1 {
			return nil, errors.New("a frame is longer than the limit")
		}
		switch {
		case err == nil:
			return line[:len(line)-1], nil
		case errors.Is(err, bufio.ErrBufferFull):
		case errors.Is(err, io.EOF) && len(line) > 0:
			return line, nil
		default:
			return nil, err
		}
	}
}
