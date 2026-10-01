package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	janzeer "github.com/janzeerorg/janzeer-sdk-go"
)

// WSOptions tunes a WebSocket client. The zero value is ready to use.
type WSOptions struct {
	// NoReconnect disables the automatic reconnect (exponential backoff, 1 s up to 30 s) and resubscription.
	NoReconnect bool
	// ReconnectDelay is the first backoff delay (default 1 s).
	ReconnectDelay time.Duration
	// OnEvent receives diagnostics: "open", "close", "error", "resubscribed". It must not block.
	OnEvent func(event string, err error)
}

// AddressActivity is one notification of an addressActivity subscription: a FINAL transaction involving a watched address.
type AddressActivity struct {
	BlockHeight int64          `json:"blockHeight"`
	BlockHash   string         `json:"blockHash"`
	Transaction janzeer.TxView `json:"transaction"`
}

// NewBlocksParams configures a newBlocks subscription.
type NewBlocksParams struct {
	// FromHeight replays committed blocks from this height, then streams live ones (exactly once per height).
	FromHeight *int64 `json:"fromHeight,omitempty"`
	// IncludeTransactions puts the transactions inline.
	IncludeTransactions bool `json:"includeTransactions"`
}

// Subscription is a live subscription. Its ID changes after a reconnect; the handler stays.
type Subscription struct {
	ws      *WS
	mu      sync.Mutex
	id      string
	kind    string
	params  any
	handler func(json.RawMessage)
}

// ID is the node's current subscription id.
func (s *Subscription) ID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.id
}

// Unsubscribe stops the subscription.
func (s *Subscription) Unsubscribe(ctx context.Context) error {
	id := s.ID()
	s.ws.mu.Lock()
	delete(s.ws.subs, id)
	connected := s.ws.conn != nil
	s.ws.mu.Unlock()
	if !connected {
		return nil
	}
	var ok bool
	return s.ws.Call(ctx, "janzeer_unsubscribe", map[string]any{"subscription": id}, &ok)
}

// WS is the JSON-RPC client over the node's WebSocket: every method of Methods, plus subscriptions.
// Handlers run one at a time on the reader goroutine: keep them fast (the node disconnects slow consumers).
type WS struct {
	Methods
	// URL ends with /rpc/ws.
	URL  string
	opts WSOptions

	mu      sync.Mutex
	conn    *websocket.Conn
	pending map[int64]chan *response
	// hooks run on the READER goroutine when the answer of a call arrives, before the next message is read. A
	// subscription is registered this way: a notification can follow its subscribe answer immediately, and it must
	// find the subscription already in subs.
	hooks  map[int64]func(*response)
	subs   map[string]*Subscription
	closed bool
	id     atomic.Int64
}

// NormalizeWSURL rewrites an http(s) origin, an /api/v1 base or a /rpc URL to ws(s)://…/rpc/ws.
func NormalizeWSURL(u string) string {
	u = strings.TrimRight(strings.TrimSpace(u), "/")
	if strings.HasPrefix(u, "http://") {
		u = "ws://" + u[len("http://"):]
	} else if strings.HasPrefix(u, "https://") {
		u = "wss://" + u[len("https://"):]
	}
	u = strings.TrimSuffix(strings.TrimSuffix(u, "/api/v1"), "/rpc")
	if !strings.HasSuffix(u, "/rpc/ws") {
		u += "/rpc/ws"
	}
	return u
}

// Dial opens the socket and returns once connected. opts may be nil.
func Dial(ctx context.Context, url string, opts *WSOptions) (*WS, error) {
	w := &WS{URL: NormalizeWSURL(url), pending: map[int64]chan *response{}, hooks: map[int64]func(*response){}, subs: map[string]*Subscription{}}
	if opts != nil {
		w.opts = *opts
	}
	if w.opts.ReconnectDelay <= 0 {
		w.opts.ReconnectDelay = time.Second
	}
	w.Methods = Methods{w}
	if err := w.open(ctx); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *WS) emit(event string, err error) {
	if w.opts.OnEvent != nil {
		w.opts.OnEvent(event, err)
	}
}

func (w *WS) open(ctx context.Context) error {
	dctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(dctx, w.URL, nil)
	if err != nil {
		return &janzeer.NetworkError{Message: fmt.Sprintf("janzeer: cannot open the WebSocket %s: %v", w.URL, err), Err: err}
	}
	conn.SetReadLimit(-1)
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		conn.Close(websocket.StatusNormalClosure, "client closed")
		return &janzeer.NetworkError{Message: "janzeer: socket closed by client"}
	}
	w.conn = conn
	w.mu.Unlock()
	go w.read(conn)
	w.emit("open", nil)
	return nil
}

func (w *WS) read(conn *websocket.Conn) {
	for {
		_, data, err := conn.Read(context.Background())
		if err != nil {
			w.mu.Lock()
			if w.conn == conn {
				w.conn = nil
			}
			pending := w.pending
			w.pending = map[int64]chan *response{}
			w.hooks = map[int64]func(*response){}
			closed := w.closed
			w.mu.Unlock()
			for _, ch := range pending {
				close(ch)
			}
			w.emit("close", err)
			if !closed && !w.opts.NoReconnect {
				go w.reconnect()
			}
			return
		}
		w.dispatch(data)
	}
}

func (w *WS) reconnect() {
	delay := w.opts.ReconnectDelay
	for {
		time.Sleep(delay)
		if delay *= 2; delay > 30*time.Second {
			delay = 30 * time.Second
		}
		w.mu.Lock()
		done := w.closed || w.conn != nil
		w.mu.Unlock()
		if done {
			return
		}
		if err := w.open(context.Background()); err != nil {
			w.emit("error", err)
			continue
		}
		w.resubscribe()
		return
	}
}

func (w *WS) resubscribe() {
	w.mu.Lock()
	old := make([]*Subscription, 0, len(w.subs))
	for _, s := range w.subs {
		old = append(old, s)
	}
	w.subs = map[string]*Subscription{}
	w.mu.Unlock()
	count := 0
	for _, s := range old {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		err := w.call(ctx, "janzeer_subscribe", map[string]any{"kind": s.kind, "params": s.params}, nil, w.register(s))
		cancel()
		if err != nil {
			w.emit("error", err)
			continue
		}
		count++
	}
	if count > 0 {
		w.emit("resubscribed", nil)
	}
}

func (w *WS) dispatch(data []byte) {
	var msgs []json.RawMessage
	if len(data) > 0 && data[0] == '[' {
		if json.Unmarshal(data, &msgs) != nil {
			return
		}
	} else {
		msgs = []json.RawMessage{data}
	}
	for _, raw := range msgs {
		var m struct {
			response
			Method string `json:"method"`
			Params *struct {
				Subscription string          `json:"subscription"`
				Result       json.RawMessage `json:"result"`
			} `json:"params"`
		}
		if json.Unmarshal(raw, &m) != nil {
			continue
		}
		if m.Method == "janzeer_subscription" && m.Params != nil {
			w.mu.Lock()
			sub := w.subs[m.Params.Subscription]
			w.mu.Unlock()
			if sub != nil && m.Params.Result != nil {
				sub.handler(m.Params.Result)
			}
			continue
		}
		id, err := m.ID.Int64()
		if err != nil {
			continue
		}
		res := m.response
		w.mu.Lock()
		ch := w.pending[id]
		delete(w.pending, id)
		hook := w.hooks[id]
		delete(w.hooks, id)
		if hook != nil {
			hook(&res) // under the lock, on the reader goroutine: see WS.hooks
		}
		w.mu.Unlock()
		if ch != nil {
			ch <- &res
		}
	}
}

// Call performs one JSON-RPC call over the socket.
func (w *WS) Call(ctx context.Context, method string, params, out any) error {
	return w.call(ctx, method, params, out, nil)
}

func (w *WS) call(ctx context.Context, method string, params, out any, hook func(*response)) error {
	id := w.id.Add(1)
	body, err := json.Marshal(request{JSONRPC: "2.0", ID: id, Method: method, Params: params})
	if err != nil {
		return err
	}
	ch := make(chan *response, 1)
	w.mu.Lock()
	conn := w.conn
	if conn == nil {
		closed := w.closed
		w.mu.Unlock()
		if closed {
			return &janzeer.NetworkError{Message: "janzeer: socket closed by client"}
		}
		return &janzeer.NetworkError{Message: "janzeer: socket is not connected (reconnecting)"}
	}
	w.pending[id] = ch
	if hook != nil {
		w.hooks[id] = hook
	}
	w.mu.Unlock()
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 65*time.Second)
		defer cancel()
	}
	if err := conn.Write(ctx, websocket.MessageText, body); err != nil {
		w.mu.Lock()
		delete(w.pending, id)
		delete(w.hooks, id)
		w.mu.Unlock()
		return &janzeer.NetworkError{Message: "janzeer: send failed: " + err.Error(), Err: err}
	}
	select {
	case res, ok := <-ch:
		if !ok {
			return &janzeer.NetworkError{Message: "janzeer: socket closed"}
		}
		if err := res.err("ws"); err != nil {
			return err
		}
		return decodeResult(res.Result, out)
	case <-ctx.Done():
		w.mu.Lock()
		delete(w.pending, id)
		delete(w.hooks, id)
		w.mu.Unlock()
		return &janzeer.NetworkError{Message: fmt.Sprintf("janzeer: RPC %s timed out", method), Err: ctx.Err()}
	}
}

// register returns the hook that puts s into subs under the id the node answered (called with w.mu held).
func (w *WS) register(s *Subscription) func(*response) {
	return func(res *response) {
		var id string
		if res.Error != nil || json.Unmarshal(res.Result, &id) != nil || id == "" {
			return
		}
		s.mu.Lock()
		s.id = id
		s.mu.Unlock()
		w.subs[id] = s
	}
}

func (w *WS) subscribe(ctx context.Context, kind string, params any, handler func(json.RawMessage)) (*Subscription, error) {
	w.mu.Lock()
	n := len(w.subs)
	w.mu.Unlock()
	if n >= janzeer.LimitSubscriptions {
		return nil, fmt.Errorf("janzeer: at most %d subscriptions per session", janzeer.LimitSubscriptions)
	}
	s := &Subscription{ws: w, kind: kind, params: params, handler: handler}
	if err := w.call(ctx, "janzeer_subscribe", map[string]any{"kind": kind, "params": params}, nil, w.register(s)); err != nil {
		return nil, err
	}
	return s, nil
}

// SubscribeNewBlocks streams committed blocks (optionally replaying from a height).
func (w *WS) SubscribeNewBlocks(ctx context.Context, params NewBlocksParams, handler func(*janzeer.BlockView)) (*Subscription, error) {
	return w.subscribe(ctx, "newBlocks", params, func(raw json.RawMessage) {
		var b janzeer.BlockView
		if json.Unmarshal(raw, &b) == nil {
			handler(&b)
		}
	})
}

// SubscribeAddressActivity notifies when a FINAL transaction involves any of the addresses (sender, recipient or
// token recipient). At most 1000 addresses.
func (w *WS) SubscribeAddressActivity(ctx context.Context, addresses []string, handler func(*AddressActivity)) (*Subscription, error) {
	if len(addresses) == 0 || len(addresses) > janzeer.LimitWatchedAddresses {
		return nil, fmt.Errorf("janzeer: 1-%d addresses", janzeer.LimitWatchedAddresses)
	}
	lower := make([]string, len(addresses))
	for i, a := range addresses {
		lower[i] = strings.ToLower(a)
	}
	return w.subscribe(ctx, "addressActivity", map[string]any{"addresses": lower}, func(raw json.RawMessage) {
		var ev AddressActivity
		if json.Unmarshal(raw, &ev) == nil {
			handler(&ev)
		}
	})
}

// Connected reports whether the socket is open right now.
func (w *WS) Connected() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.conn != nil
}

// Close closes the socket; there is no reconnect. Pending calls fail and subscriptions are dropped.
func (w *WS) Close() error {
	w.mu.Lock()
	w.closed = true
	conn := w.conn
	w.conn = nil
	w.subs = map[string]*Subscription{}
	w.mu.Unlock()
	if conn == nil {
		return nil
	}
	err := conn.Close(websocket.StatusNormalClosure, "client closed")
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}
