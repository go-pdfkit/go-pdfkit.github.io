// Copyright (c) the go-pdfkit authors.
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// ⛔ THE SECOND DEVTOOLS CLIENT IN THE FLEET, AND IT SAYS SO. The first is in
// go-pdfkit/app/browsercheck, which drives the workbench. Neither is extracted
// yet because two callers is where an abstraction is guessed at rather than
// known; this one is deliberately the SMALL half — connect, emulate, evaluate,
// screenshot — so that moving it into a shared module later is a move and not
// a rewrite. A third caller is the moment to do that.
//
// cmd/pagecheck next door does NOT use this: it asks Chrome for one screenshot
// and judges it, which needs no protocol at all. What needs the protocol is
// changing something about the page AFTER it has loaded and asking again, and
// that is the whole point of this check.

// browserStartup bounds the wait for the line Chrome prints when it opens its
// debugging port. A machine with no desktop prints a page of complaints first.
const browserStartup = 90 * time.Second

// devtoolsURL reads the ws:// line Chrome writes to stderr on start-up.
func devtoolsURL(stderr io.Reader) (string, error) {
	var buf strings.Builder
	tmp := make([]byte, 512)
	deadline := time.Now().Add(browserStartup)
	for time.Now().Before(deadline) {
		n, err := stderr.Read(tmp)
		buf.Write(tmp[:n])
		if i := strings.Index(buf.String(), "ws://"); i >= 0 {
			if line, _, ok := strings.Cut(buf.String()[i:], "\n"); ok {
				return strings.TrimRight(line, "\r"), nil
			}
		}
		if err != nil {
			return "", fmt.Errorf("chrome said: %s", buf.String())
		}
	}
	return "", fmt.Errorf("chrome never printed a debugging url: %s", buf.String())
}

// A conn is one DevTools connection: commands numbered, replies matched by
// number.
//
// ⛔ THE READER TAKES THE LOCK TOO. The map is written by the caller's
// goroutine, which registers a reply channel, and read and deleted by the
// reader goroutine when the reply lands. Guarding only one of the two is not
// a smaller version of guarding both — Go's runtime throws "concurrent map
// writes" and the process dies mid-check with the browser still open.
type conn struct {
	ws *websocket.Conn

	mu      sync.Mutex
	id      int
	replies map[int]chan json.RawMessage
}

func dial(ctx context.Context, url string) (*conn, error) {
	ws, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		return nil, err
	}
	// A screenshot of a 1280×900 page arrives as one base64 message.
	ws.SetReadLimit(64 << 20)
	c := &conn{ws: ws, replies: map[int]chan json.RawMessage{}}
	go c.pump(ctx)
	return c, nil
}

func (c *conn) close() { _ = c.ws.Close(websocket.StatusNormalClosure, "") }

func (c *conn) pump(ctx context.Context) {
	for {
		_, data, err := c.ws.Read(ctx)
		if err != nil {
			return
		}
		var msg struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  json.RawMessage `json:"error"`
		}
		if json.Unmarshal(data, &msg) != nil || msg.ID == 0 {
			continue // an event; this check waits for replies only
		}
		c.mu.Lock()
		ch, ok := c.replies[msg.ID]
		delete(c.replies, msg.ID)
		c.mu.Unlock()
		if !ok {
			continue
		}
		if msg.Error != nil {
			ch <- json.RawMessage(`{"__error":` + string(msg.Error) + `}`)
			continue
		}
		ch <- msg.Result
	}
}

// call sends one command and waits for its reply.
func (c *conn) call(ctx context.Context, method string, params map[string]any, session string) (json.RawMessage, error) {
	ch := make(chan json.RawMessage, 1)
	c.mu.Lock()
	c.id++
	id := c.id
	c.replies[id] = ch
	c.mu.Unlock()
	// Whatever happens next this entry must not outlive the call: a write that
	// fails, or a context that ends, would leave pump holding a channel nobody
	// will ever read.
	defer func() {
		c.mu.Lock()
		delete(c.replies, id)
		c.mu.Unlock()
	}()

	req := map[string]any{"id": id, "method": method}
	if params != nil {
		req["params"] = params
	}
	if session != "" {
		req["sessionId"] = session
	}
	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if err := c.ws.Write(ctx, websocket.MessageText, data); err != nil {
		return nil, err
	}
	select {
	case r := <-ch:
		var e struct {
			Err json.RawMessage `json:"__error"`
		}
		if json.Unmarshal(r, &e) == nil && e.Err != nil {
			return nil, fmt.Errorf("%s: %s", method, e.Err)
		}
		return r, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("%s: %w", method, ctx.Err())
	}
}

// attach opens a session on the first page target Chrome offers.
func attach(ctx context.Context, c *conn) (string, error) {
	r, err := c.call(ctx, "Target.getTargets", nil, "")
	if err != nil {
		return "", err
	}
	var targets struct {
		TargetInfos []struct {
			TargetID string `json:"targetId"`
			Type     string `json:"type"`
		} `json:"targetInfos"`
	}
	if err := json.Unmarshal(r, &targets); err != nil {
		return "", err
	}
	for _, t := range targets.TargetInfos {
		if t.Type != "page" {
			continue
		}
		r, err := c.call(ctx, "Target.attachToTarget",
			map[string]any{"targetId": t.TargetID, "flatten": true}, "")
		if err != nil {
			return "", err
		}
		var s struct {
			SessionID string `json:"sessionId"`
		}
		if err := json.Unmarshal(r, &s); err != nil {
			return "", err
		}
		return s.SessionID, nil
	}
	return "", fmt.Errorf("chrome offered no page to attach to")
}

// scheme emulates the reader's system setting. This is the whole reason the
// check speaks the protocol: there is no command-line flag that says "the
// machine is in dark mode", and the page resolves "system" through exactly
// this media feature.
func scheme(ctx context.Context, c *conn, session, want string) error {
	_, err := c.call(ctx, "Emulation.setEmulatedMedia", map[string]any{
		"features": []any{map[string]any{"name": "prefers-color-scheme", "value": want}},
	}, session)
	return err
}

// eval runs an expression and returns its value as a string.
func eval(ctx context.Context, c *conn, session, js string) (string, error) {
	r, err := c.call(ctx, "Runtime.evaluate", map[string]any{
		"expression":    js,
		"returnByValue": true,
		"awaitPromise":  true,
	}, session)
	if err != nil {
		return "", err
	}
	var res struct {
		Result struct {
			Value any `json:"value"`
		} `json:"result"`
		Exception json.RawMessage `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(r, &res); err != nil {
		return "", err
	}
	if res.Exception != nil {
		return "", fmt.Errorf("%s: %s", js, res.Exception)
	}
	return fmt.Sprint(res.Result.Value), nil
}

// shot captures the page as PNG bytes.
func shot(ctx context.Context, c *conn, session string) ([]byte, error) {
	r, err := c.call(ctx, "Page.captureScreenshot", map[string]any{"format": "png"}, session)
	if err != nil {
		return nil, err
	}
	var res struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(r, &res); err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(res.Data)
}

// until polls cond until it holds or the patience runs out. The page is a
// WebAssembly module repainting on its own clock, so every assertion about it
// is "eventually", and a check that read once would be a race.
func until(d time.Duration, what string, cond func() (bool, error)) error {
	deadline := time.Now().Add(d)
	for {
		ok, err := cond()
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for %s", what)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
