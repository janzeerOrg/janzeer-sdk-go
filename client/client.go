// Package client is the client of the node's REST API (/api/v1/).
//
//	c := client.New("https://onion.janzeer.org")
//	balance, err := c.Balance(ctx, address)        // "12.50000000": a decimal string, never a float
//
// Every call takes a context. A Client is safe for concurrent use.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	janzeer "github.com/janzeerorg/janzeer-sdk-go"
)

// Envelope is the metadata of the REST envelope {timestamp, version, payload}.
type Envelope struct {
	Timestamp int64
	Version   string
}

// Client talks to one node.
type Client struct {
	// BaseURL ends with /api/v1/.
	BaseURL string
	// HTTP is the underlying client (default: 15 s timeout).
	HTTP *http.Client
	// Header is added to every request.
	Header http.Header
	// NoRetryOnRateLimit disables the single retry after an HTTP 429.
	NoRetryOnRateLimit bool

	mu   sync.Mutex
	last *Envelope
}

// New creates a client. A missing /api/v1/ suffix is added: New("http://localhost:7019") works.
func New(baseURL string) *Client {
	u := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if !strings.HasSuffix(u, "/api/v1") {
		u += "/api/v1"
	}
	return &Client{BaseURL: u + "/", HTTP: &http.Client{Timeout: 15 * time.Second}, Header: http.Header{}}
}

// LastEnvelope returns the envelope of the last successful response (nil before the first one).
func (c *Client) LastEnvelope() *Envelope {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.last
}

// errNotFound is used internally to turn a 404 into "no value".
var errNotFound = errors.New("not found")

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body any, out any, notFoundOK bool) error {
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return err
		}
	}
	u := c.BaseURL + strings.TrimLeft(path, "/")
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		for k, v := range c.Header {
			req.Header[k] = v
		}
		req.Header.Set("Accept", "application/json")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		res, err := c.HTTP.Do(req)
		if err != nil {
			return &janzeer.NetworkError{Message: fmt.Sprintf("janzeer: cannot reach the node at %s: %v", c.BaseURL, err), Err: err}
		}
		raw, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			return &janzeer.NetworkError{Message: "janzeer: reading the response failed: " + err.Error(), Err: err}
		}
		if res.StatusCode == http.StatusNotFound && notFoundOK {
			return errNotFound
		}
		if res.StatusCode < 200 || res.StatusCode > 299 {
			e := janzeer.RESTErrorFrom(res.StatusCode, raw, res.Header.Get("Retry-After"))
			var rl *janzeer.RateLimitedError
			if errors.As(e, &rl) && !c.NoRetryOnRateLimit && attempt == 0 {
				select {
				case <-time.After(rl.RetryAfter):
					continue
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return e
		}
		var env struct {
			Timestamp json.Number     `json:"timestamp"`
			Version   string          `json:"version"`
			Payload   json.RawMessage `json:"payload"`
		}
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		if err := dec.Decode(&env); err != nil {
			return &janzeer.NetworkError{Message: fmt.Sprintf("janzeer: node returned non-JSON (%d)", res.StatusCode), Err: err}
		}
		ts, _ := env.Timestamp.Int64()
		c.mu.Lock()
		c.last = &Envelope{Timestamp: ts, Version: env.Version}
		c.mu.Unlock()
		if out == nil || len(env.Payload) == 0 || string(env.Payload) == "null" {
			return nil
		}
		return json.Unmarshal(env.Payload, out)
	}
}

// Get performs GET path?query and decodes the envelope payload into out. found is false on HTTP 404.
func (c *Client) Get(ctx context.Context, path string, query url.Values, out any) (found bool, err error) {
	err = c.do(ctx, http.MethodGet, path, query, nil, out, true)
	if errors.Is(err, errNotFound) {
		return false, nil
	}
	return err == nil, err
}

// Post performs POST path with a JSON body and decodes the envelope payload into out (out may be nil).
func (c *Client) Post(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, http.MethodPost, path, nil, body, out, false)
}

// ---- node ----

// Info is GET info: node key, network id, genesis hash, versions, sync status.
func (c *Client) Info(ctx context.Context) (*janzeer.NodeInfo, error) {
	var out janzeer.NodeInfo
	return &out, c.do(ctx, http.MethodGet, "info", nil, nil, &out, false)
}

// Genesis returns the network's public genesis document as raw JSON.
func (c *Client) Genesis(ctx context.Context) (json.RawMessage, error) {
	var out json.RawMessage
	err := c.do(ctx, http.MethodGet, "info/genesis", nil, nil, &out, false)
	return out, err
}

// ---- wallet ----

// Balance is the spendable balance (committed minus pending debits) as a decimal string; "0" for an unknown address.
func (c *Client) Balance(ctx context.Context, address string) (string, error) {
	var out janzeer.Amount
	found, err := c.Get(ctx, "wallets/"+address, nil, &out)
	if err != nil {
		return "", err
	}
	if !found || out == "" {
		return "0", nil
	}
	return out.String(), nil
}

// Nonce is the next nonce to sign with (committed nonce + pending count); 0 for an unknown address.
func (c *Client) Nonce(ctx context.Context, address string) (int64, error) {
	var out int64
	_, err := c.Get(ctx, "wallets/"+address+"/nonce", nil, &out)
	return out, err
}

// ---- transactions ----

// Submit sends a signed transaction (HTTP 201) and returns the node's echo. A refusal is a *janzeer.TxRejectedError
// (a nonce problem additionally a *janzeer.NonceMismatchError).
func (c *Client) Submit(ctx context.Context, tx *janzeer.SignedTx) (*janzeer.Transaction, error) {
	var out janzeer.Transaction
	if err := c.Post(ctx, tx.RESTPath(), tx.Body(), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListOptions pages and filters a list. Zero values are omitted.
type ListOptions struct {
	Page, Size    int
	SortBy        string
	SortDirection string
	// Address filters by sender or recipient (rewards: by recipient).
	Address string
	// Unconfirmed lists the mempool instead of the ledger.
	Unconfirmed bool
	// TokenID filters token transactions; NodeKey filters validator registrations.
	TokenID, NodeKey string
}

func (o ListOptions) query() url.Values {
	q := url.Values{}
	if o.Page > 0 {
		q.Set("page", strconv.Itoa(o.Page))
	}
	if o.Size > 0 {
		q.Set("size", strconv.Itoa(o.Size))
	}
	for k, v := range map[string]string{"sortBy": o.SortBy, "sortDirection": o.SortDirection, "address": o.Address, "tokenId": o.TokenID, "nodeKey": o.NodeKey} {
		if v != "" {
			q.Set(k, v)
		}
	}
	if o.Unconfirmed {
		q.Set("unconfirmed", "true")
	}
	return q
}

// Collection names a REST transaction collection.
type Collection string

// Transaction collections.
const (
	Transfers      Collection = "transactions/transfers"
	ValidatorTxs   Collection = "transactions/validators"
	ExitValidators Collection = "transactions/exit-validators"
	TokenTxs       Collection = "transactions/tokens"
	Rewards        Collection = "transactions/rewards"
)

// List returns one page of a transaction collection.
func (c *Client) List(ctx context.Context, col Collection, o ListOptions) (*janzeer.Page[janzeer.Transaction], error) {
	var out janzeer.Page[janzeer.Transaction]
	_, err := c.Get(ctx, string(col), o.query(), &out)
	return &out, err
}

// Transaction looks a transaction up in one collection; nil (and no error) when the node does not know the hash.
func (c *Client) Transaction(ctx context.Context, col Collection, hash string) (*janzeer.Transaction, error) {
	var out janzeer.Transaction
	found, err := c.Get(ctx, string(col)+"/"+hash, nil, &out)
	if err != nil || !found {
		return nil, err
	}
	return &out, nil
}

// Receipt is the execution receipt of a committed transaction; nil when unknown or still pending.
func (c *Client) Receipt(ctx context.Context, hash string) (*janzeer.Receipt, error) {
	var out janzeer.Receipt
	found, err := c.Get(ctx, "transactions/"+hash+"/receipt", nil, &out)
	if err != nil || !found {
		return nil, err
	}
	return &out, nil
}

// Validators lists every registered validator, or with active the current epoch's producer set.
func (c *Client) Validators(ctx context.Context, active bool, o ListOptions) (*janzeer.Page[janzeer.Validator], error) {
	path := "validators"
	if active {
		path += "/active"
	}
	var out janzeer.Page[janzeer.Validator]
	_, err := c.Get(ctx, path, o.query(), &out)
	return &out, err
}

// WaitForFinality polls the transaction endpoints until hash is in a committed block or ctx ends (use a context with
// a deadline). On expiry it returns a *janzeer.FinalityTimeoutError. Prefer rpc.WaitForFinality when JSON-RPC is
// available: it long-polls instead.
func (c *Client) WaitForFinality(ctx context.Context, hash string, poll time.Duration) (*janzeer.Transaction, error) {
	if poll <= 0 {
		poll = 1500 * time.Millisecond
	}
	for {
		for _, col := range []Collection{Transfers, TokenTxs, ValidatorTxs, ExitValidators} {
			t, err := c.Transaction(ctx, col, hash)
			if err != nil {
				if ctx.Err() != nil {
					return nil, &janzeer.FinalityTimeoutError{Hash: hash}
				}
				return nil, err
			}
			if t != nil {
				if t.Final() {
					return t, nil
				}
				break
			}
		}
		select {
		case <-ctx.Done():
			return nil, &janzeer.FinalityTimeoutError{Hash: hash}
		case <-time.After(poll):
		}
	}
}
