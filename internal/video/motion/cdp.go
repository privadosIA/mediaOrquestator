package motion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/coder/websocket"
)

// cdp is a minimal Chrome DevTools Protocol client over one WebSocket: numbered
// calls with their answers, and a handler for the events the caller subscribes to.
type cdp struct {
	conn   *websocket.Conn
	nextID atomic.Int64

	mu      sync.Mutex
	pending map[int64]chan cdpReply
	onEvent func(method string, params json.RawMessage)

	done chan struct{}
	err  error // why the reader stopped, set before done is closed
}

type cdpReply struct {
	Result json.RawMessage
	Err    *cdpError
}

type cdpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *cdpError) Error() string { return fmt.Sprintf("devtools: %s (%d)", e.Message, e.Code) }

// maxMessage bounds one protocol message; a full-HD PNG screenshot in base64 is a few MB.
const maxMessage = 128 << 20

func dialCDP(ctx context.Context, url string, onEvent func(method string, params json.RawMessage)) (*cdp, error) {
	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot connect to the browser: %w", err)
	}
	conn.SetReadLimit(maxMessage)
	c := &cdp{conn: conn, pending: map[int64]chan cdpReply{}, onEvent: onEvent, done: make(chan struct{})}
	go c.read()
	return c, nil
}

func (c *cdp) read() {
	defer close(c.done)
	for {
		_, data, err := c.conn.Read(context.Background())
		if err != nil {
			c.mu.Lock()
			c.err = err
			for id, ch := range c.pending {
				ch <- cdpReply{Err: &cdpError{Message: "the browser closed the connection"}}
				delete(c.pending, id)
			}
			c.mu.Unlock()
			return
		}
		var m struct {
			ID     int64           `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
			Result json.RawMessage `json:"result"`
			Error  *cdpError       `json:"error"`
		}
		if json.Unmarshal(data, &m) != nil {
			continue
		}
		if m.ID != 0 {
			c.mu.Lock()
			ch := c.pending[m.ID]
			delete(c.pending, m.ID)
			c.mu.Unlock()
			if ch != nil {
				ch <- cdpReply{Result: m.Result, Err: m.Error}
			}
			continue
		}
		if m.Method != "" && c.onEvent != nil {
			c.onEvent(m.Method, m.Params)
		}
	}
}

// call sends one command and waits for its answer, the context's end, or the
// connection's. A context that ends leaves the browser running: the caller kills it.
func (c *cdp) call(ctx context.Context, method string, params any, out any) error {
	id := c.nextID.Add(1)
	ch := make(chan cdpReply, 1)
	c.mu.Lock()
	if c.err != nil {
		c.mu.Unlock()
		return errors.New("the browser closed the connection")
	}
	c.pending[id] = ch
	c.mu.Unlock()
	body, err := json.Marshal(map[string]any{"id": id, "method": method, "params": params})
	if err != nil {
		return err
	}
	if err := c.conn.Write(ctx, websocket.MessageText, body); err != nil {
		return fmt.Errorf("%s: %w", method, err)
	}
	select {
	case r := <-ch:
		if r.Err != nil {
			return fmt.Errorf("%s: %w", method, r.Err)
		}
		if out != nil && len(r.Result) > 0 {
			return json.Unmarshal(r.Result, out)
		}
		return nil
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return fmt.Errorf("%s: %w", method, ctx.Err())
	}
}

func (c *cdp) close() { _ = c.conn.CloseNow() }
