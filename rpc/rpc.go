// Package rpc is the JSON-RPC 2.0 client of the node: over HTTP (Client, POST /rpc) and over WebSocket (WS, /rpc/ws)
// with live subscriptions. Both expose the same typed janzeer_* methods through the embedded Methods.
//
//	r := rpc.New("https://onion.janzeer.org")
//	acct, err := r.GetAccount(ctx, address)
//	view, err := rpc.WaitForFinality(ctx, r, hash)
package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	janzeer "github.com/janzeerorg/janzeer-sdk-go"
)

// Caller performs one JSON-RPC call and decodes the result into out (out may be nil). params is a struct or map
// (by name), a slice (by position) or nil.
type Caller interface {
	Call(ctx context.Context, method string, params, out any) error
}

// Methods are the typed janzeer_* methods over any Caller.
type Methods struct{ c Caller }

// NewMethods wraps a custom transport.
func NewMethods(c Caller) Methods { return Methods{c} }

// Call sends any method (use it for a method this SDK release does not wrap).
func (m Methods) Call(ctx context.Context, method string, params, out any) error {
	return m.c.Call(ctx, method, params, out)
}

type addr struct {
	Address string `json:"address"`
}

// GetInfo is janzeer_getInfo (chain id, versions, tip, sync status, peers) as raw JSON.
func (m Methods) GetInfo(ctx context.Context) (json.RawMessage, error) {
	return m.raw(ctx, "janzeer_getInfo", nil)
}

// GetChainSpec is janzeer_getChainSpec (consensus parameters, fees, supply) as raw JSON.
func (m Methods) GetChainSpec(ctx context.Context) (json.RawMessage, error) {
	return m.raw(ctx, "janzeer_getChainSpec", nil)
}

// GetStats is janzeer_getStats as raw JSON.
func (m Methods) GetStats(ctx context.Context) (json.RawMessage, error) {
	return m.raw(ctx, "janzeer_getStats", nil)
}

// GetEpoch is janzeer_getEpoch as raw JSON.
func (m Methods) GetEpoch(ctx context.Context) (json.RawMessage, error) {
	return m.raw(ctx, "janzeer_getEpoch", nil)
}

func (m Methods) raw(ctx context.Context, method string, params any) (json.RawMessage, error) {
	var out json.RawMessage
	err := m.c.Call(ctx, method, params, &out)
	return out, err
}

// GetAccount returns balance, nonces and pending count in one call.
func (m Methods) GetAccount(ctx context.Context, address string) (*janzeer.AccountView, error) {
	var out janzeer.AccountView
	return &out, m.c.Call(ctx, "janzeer_getAccount", addr{address}, &out)
}

// GetBalance returns the spendable balance as a decimal string.
func (m Methods) GetBalance(ctx context.Context, address string) (string, error) {
	var out janzeer.Amount
	err := m.c.Call(ctx, "janzeer_getBalance", addr{address}, &out)
	return out.String(), err
}

// GetNonce returns the next nonce (committed + pending).
func (m Methods) GetNonce(ctx context.Context, address string) (int64, error) {
	var out int64
	err := m.c.Call(ctx, "janzeer_getNonce", addr{address}, &out)
	return out, err
}

// Send submits a signed transaction through its type's janzeer_send* method.
func (m Methods) Send(ctx context.Context, tx *janzeer.SignedTx) (*janzeer.SendResult, error) {
	var out janzeer.SendResult
	if err := m.c.Call(ctx, tx.RPCMethod(), tx.Body(), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetTransactionByHash returns the unified view of any transaction; Status is UNKNOWN for a hash the node never saw.
func (m Methods) GetTransactionByHash(ctx context.Context, hash string) (*janzeer.TxView, error) {
	var out janzeer.TxView
	return &out, m.c.Call(ctx, "janzeer_getTransactionByHash", map[string]any{"hash": hash}, &out)
}

// WaitForFinalityOnce is the node's long-poll (at most 60 s per call). Prefer the WaitForFinality function.
func (m Methods) WaitForFinalityOnce(ctx context.Context, hash string, timeout time.Duration) (*janzeer.TxView, error) {
	var out janzeer.TxView
	return &out, m.c.Call(ctx, "janzeer_waitForFinality", map[string]any{"hash": hash, "timeoutMs": timeout.Milliseconds()}, &out)
}

// EstimateFee returns the fee rule of a transaction kind ("transfer", "registerValidator", "tokenCreate", …).
func (m Methods) EstimateFee(ctx context.Context, kind string) (*janzeer.FeeEstimate, error) {
	var out janzeer.FeeEstimate
	return &out, m.c.Call(ctx, "janzeer_estimateFee", map[string]any{"kind": kind}, &out)
}

// GetTip returns the newest committed block.
func (m Methods) GetTip(ctx context.Context) (*janzeer.BlockView, error) {
	var out janzeer.BlockView
	return &out, m.c.Call(ctx, "janzeer_getTip", nil, &out)
}

// GetBlockByNumber returns the block at a height.
func (m Methods) GetBlockByNumber(ctx context.Context, height int64, includeTransactions bool) (*janzeer.BlockView, error) {
	var out janzeer.BlockView
	return &out, m.c.Call(ctx, "janzeer_getBlockByNumber", map[string]any{"height": height, "includeTransactions": includeTransactions}, &out)
}

// GetBlockByHash returns the block with a hash.
func (m Methods) GetBlockByHash(ctx context.Context, hash string, includeTransactions bool) (*janzeer.BlockView, error) {
	var out janzeer.BlockView
	return &out, m.c.Call(ctx, "janzeer_getBlockByHash", map[string]any{"hash": hash, "includeTransactions": includeTransactions}, &out)
}

// GetBlocks returns the blocks of a height range, ascending (at most 100 per call).
func (m Methods) GetBlocks(ctx context.Context, fromHeight, toHeight int64, includeTransactions bool) ([]janzeer.BlockView, error) {
	var out []janzeer.BlockView
	err := m.c.Call(ctx, "janzeer_getBlocks", map[string]any{"fromHeight": fromHeight, "toHeight": toHeight, "includeTransactions": includeTransactions}, &out)
	return out, err
}

// GetActiveValidators returns the current epoch's producer set.
func (m Methods) GetActiveValidators(ctx context.Context) ([]janzeer.ValidatorInfo, error) {
	var out []janzeer.ValidatorInfo
	err := m.c.Call(ctx, "janzeer_getActiveValidators", nil, &out)
	return out, err
}

// ListValidators returns one page of every registered validator (page 0-based, size 1-100; 0 = node default).
func (m Methods) ListValidators(ctx context.Context, page, size int) (*janzeer.Page[janzeer.ValidatorInfo], error) {
	p := map[string]any{}
	if page > 0 {
		p["page"] = page
	}
	if size > 0 {
		p["size"] = size
	}
	var out janzeer.Page[janzeer.ValidatorInfo]
	return &out, m.c.Call(ctx, "janzeer_listValidators", p, &out)
}

// GetValidator returns one validator by its node key.
func (m Methods) GetValidator(ctx context.Context, key string) (*janzeer.ValidatorInfo, error) {
	var out janzeer.ValidatorInfo
	return &out, m.c.Call(ctx, "janzeer_getValidator", map[string]any{"key": key}, &out)
}

// WaitForFinality waits until hash is in a committed block and returns its FINAL view. It chains the node's
// long-poll in slices of at most 60 s until ctx ends (use a context with a deadline; without one it waits 3 minutes).
// On expiry it returns a *janzeer.FinalityTimeoutError carrying the last view.
func WaitForFinality(ctx context.Context, m interface {
	WaitForFinalityOnce(context.Context, string, time.Duration) (*janzeer.TxView, error)
}, hash string) (*janzeer.TxView, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 3*time.Minute)
		defer cancel()
	}
	var last *janzeer.TxView
	for ctx.Err() == nil {
		deadline, _ := ctx.Deadline()
		slice := time.Until(deadline)
		if slice > janzeer.LimitWaitForFinalityMs*time.Millisecond {
			slice = janzeer.LimitWaitForFinalityMs * time.Millisecond
		}
		if slice < time.Second {
			slice = time.Second
		}
		v, err := m.WaitForFinalityOnce(ctx, hash, slice)
		var netErr *janzeer.NetworkError
		switch {
		case err == nil:
			last = v
			if v.Status == "FINAL" {
				return v, nil
			}
		case janzeer.IsRPCCode(err, janzeer.RPCNotFound):
			last = nil
		case errors.As(err, &netErr):
			select {
			case <-ctx.Done():
			case <-time.After(time.Second):
			}
		default:
			return nil, err
		}
	}
	return nil, &janzeer.FinalityTimeoutError{Hash: hash, Last: last}
}

// ---- HTTP transport ----

// Client is the JSON-RPC client over HTTP. It is safe for concurrent use.
type Client struct {
	Methods
	// URL ends with /rpc.
	URL string
	// HTTP is the underlying client (default: 65 s timeout, above the node's 60 s finality long-poll).
	HTTP *http.Client
	// Header is added to every request.
	Header http.Header
	// NoRetryOnRateLimit disables the single retry after an HTTP 429.
	NoRetryOnRateLimit bool
	id                 atomic.Int64
}

// NormalizeURL rewrites a bare origin or an /api/v1 base to …/rpc.
func NormalizeURL(u string) string {
	u = strings.TrimSuffix(strings.TrimRight(strings.TrimSpace(u), "/"), "/api/v1")
	if !strings.HasSuffix(u, "/rpc") {
		u += "/rpc"
	}
	return u
}

// New creates a JSON-RPC client: New("https://onion.janzeer.org").
func New(url string) *Client {
	c := &Client{URL: NormalizeURL(url), HTTP: &http.Client{Timeout: 65 * time.Second}, Header: http.Header{}}
	c.Methods = Methods{c}
	return c
}

type request struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type response struct {
	ID     json.Number     `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	} `json:"error"`
}

func (r *response) err(transport string) error {
	if r.Error == nil {
		return nil
	}
	return janzeer.RPCErrorFrom(r.Error.Code, r.Error.Message, r.Error.Data, transport)
}

func decodeResult(raw json.RawMessage, out any) error {
	if out == nil || len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func (c *Client) post(ctx context.Context, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		for k, v := range c.Header {
			req.Header[k] = v
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		res, err := c.HTTP.Do(req)
		if err != nil {
			return nil, &janzeer.NetworkError{Message: fmt.Sprintf("janzeer: cannot reach the node at %s: %v", c.URL, err), Err: err}
		}
		raw, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			return nil, &janzeer.NetworkError{Message: "janzeer: reading the response failed: " + err.Error(), Err: err}
		}
		if res.StatusCode == http.StatusTooManyRequests {
			seconds, perr := strconv.ParseFloat(res.Header.Get("Retry-After"), 64)
			if perr != nil {
				seconds = 1
			}
			wait := time.Duration(seconds * float64(time.Second))
			if c.NoRetryOnRateLimit || attempt > 0 {
				return nil, &janzeer.RateLimitedError{Message: "Rate limit exceeded", RetryAfter: wait, Transport: "rpc"}
			}
			select {
			case <-time.After(wait):
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return raw, nil
	}
}

// Call performs one JSON-RPC call.
func (c *Client) Call(ctx context.Context, method string, params, out any) error {
	raw, err := c.post(ctx, request{JSONRPC: "2.0", ID: c.id.Add(1), Method: method, Params: params})
	if err != nil {
		return err
	}
	var res response
	if err := json.Unmarshal(raw, &res); err != nil {
		return &janzeer.NetworkError{Message: "janzeer: malformed JSON-RPC response", Err: err}
	}
	if err := res.err("rpc"); err != nil {
		return err
	}
	return decodeResult(res.Result, out)
}

// BatchCall is one entry of a batch: Out receives the result, Err the entry's own error.
type BatchCall struct {
	Method string
	Params any
	Out    any
	Err    error
}

// Batch sends several calls in one HTTP round trip (node cap: 50). Each entry's Out or Err is filled; the returned
// error is non-nil only when the batch as a whole failed.
func (c *Client) Batch(ctx context.Context, calls []*BatchCall) error {
	if len(calls) == 0 {
		return nil
	}
	reqs := make([]request, len(calls))
	for i, call := range calls {
		reqs[i] = request{JSONRPC: "2.0", ID: c.id.Add(1), Method: call.Method, Params: call.Params}
	}
	raw, err := c.post(ctx, reqs)
	if err != nil {
		return err
	}
	var list []response
	if err := json.Unmarshal(raw, &list); err != nil {
		var single response // a batch-level error (e.g. "Batch too large") comes back as one object
		if json.Unmarshal(raw, &single) == nil && single.Error != nil {
			return single.err("rpc")
		}
		return &janzeer.NetworkError{Message: "janzeer: malformed JSON-RPC batch response", Err: err}
	}
	byID := make(map[string]*response, len(list))
	for i := range list {
		byID[list[i].ID.String()] = &list[i]
	}
	for i, call := range calls {
		r, ok := byID[strconv.FormatInt(reqs[i].ID, 10)]
		switch {
		case !ok:
			call.Err = &janzeer.RPCError{Code: janzeer.RPCInternal, Message: "missing batch response"}
		case r.Error != nil:
			call.Err = r.err("rpc")
		default:
			call.Err = decodeResult(r.Result, call.Out)
		}
	}
	return nil
}
