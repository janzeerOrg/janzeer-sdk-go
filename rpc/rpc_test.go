package rpc_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	janzeer "github.com/janzeerorg/janzeer-sdk-go"
	"github.com/janzeerorg/janzeer-sdk-go/rpc"
)

const addr = "0x74d2bedc03ae5deb6fc63fbf7bb87e86f034b274"

type req struct {
	ID     int64           `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

func answer(r req) string {
	id, _ := json.Marshal(r.ID)
	ok := func(result string) string { return `{"jsonrpc":"2.0","id":` + string(id) + `,"result":` + result + `}` }
	fail := func(code int, msg, data string) string {
		c, _ := json.Marshal(code)
		m, _ := json.Marshal(msg)
		return `{"jsonrpc":"2.0","id":` + string(id) + `,"error":{"code":` + string(c) + `,"message":` + string(m) + `,"data":` + data + `}}`
	}
	switch r.Method {
	case "janzeer_getBalance":
		return ok("12.50000000")
	case "janzeer_getNonce":
		return ok("9")
	case "janzeer_getAccount":
		return ok(`{"address":"` + addr + `","balance":99998.74000000,"committedBalance":100000,"nextNonce":3,"committedNonce":2,"pendingCount":1,"isValidatorWallet":false}`)
	case "janzeer_getTip":
		return ok(`{"type":"main","height":42,"hash":"ab","transactionsCount":0}`)
	case "janzeer_sendTransfer":
		return fail(-32001, "Invalid nonce for "+addr+": expected 1, got 11", `{"type":"INVALID_NONCE"}`)
	case "janzeer_getValidator":
		return fail(-32000, "validator not found", "null")
	case "janzeer_subscribe":
		return ok(`"00112233aabbccdd"`)
	case "janzeer_unsubscribe":
		return ok("true")
	}
	return fail(-32602, "bad params", `["address"]`)
}

func TestHTTPCallsErrorsAndBatch(t *testing.T) {
	if rpc.NormalizeURL("http://h:7019/api/v1/") != "http://h:7019/rpc" || rpc.NormalizeURL("http://h:7019") != "http://h:7019/rpc" {
		t.Fatal("URL normalization")
	}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var batch []req
		if json.Unmarshal(raw, &batch) == nil {
			out := "["
			for i := len(batch) - 1; i >= 0; i-- { // answer in reverse order: the client matches by id
				out += answer(batch[i])
				if i > 0 {
					out += ","
				}
			}
			io.WriteString(w, out+"]")
			return
		}
		var one req
		_ = json.Unmarshal(raw, &one)
		io.WriteString(w, answer(one))
	}))
	defer s.Close()
	r, ctx := rpc.New(s.URL), context.Background()

	if b, err := r.GetBalance(ctx, addr); err != nil || b != "12.50000000" {
		t.Fatalf("balance %q %v", b, err)
	}
	if n, err := r.GetNonce(ctx, addr); err != nil || n != 9 {
		t.Fatalf("nonce %d %v", n, err)
	}
	a, err := r.GetAccount(ctx, addr)
	if err != nil || a.Balance != "99998.74000000" || a.NextNonce != 3 || a.PendingCount != 1 {
		t.Fatalf("account %+v %v", a, err)
	}
	if tip, err := r.GetTip(ctx); err != nil || tip.Height != 42 {
		t.Fatalf("tip %v", err)
	}
	if _, err := r.GetValidator(ctx, "02ab"); !janzeer.IsRPCCode(err, janzeer.RPCNotFound) {
		t.Fatalf("not found: %v", err)
	}
	var raw json.RawMessage
	err = r.Call(ctx, "janzeer_somethingElse", map[string]any{"x": 1}, &raw)
	var re *janzeer.RPCError
	if !errors.As(err, &re) || re.Code != janzeer.RPCInvalidParams || string(re.Data) != `["address"]` {
		t.Fatalf("invalid params: %v", err)
	}
	acct, _ := janzeer.NewRandomAccount()
	utx, _ := janzeer.NewTransfer(janzeer.Transfer{Common: janzeer.Common{From: acct.Address(), Nonce: 11}, To: addr, Amount: "1"})
	tx, _ := acct.SignTx(utx)
	var nm *janzeer.NonceMismatchError
	if _, err := r.Send(ctx, tx); !errors.As(err, &nm) || nm.Expected != 1 || nm.RPCCode != janzeer.RPCRejected || nm.Transport != "rpc" {
		t.Fatalf("send: %v", err)
	}

	var nonce int64
	var bal janzeer.Amount
	calls := []*rpc.BatchCall{
		{Method: "janzeer_getNonce", Params: map[string]any{"address": addr}, Out: &nonce},
		{Method: "janzeer_getValidator", Params: map[string]any{"key": "02ab"}},
		{Method: "janzeer_getBalance", Params: map[string]any{"address": addr}, Out: &bal},
	}
	if err := r.Batch(ctx, calls); err != nil {
		t.Fatal(err)
	}
	if nonce != 9 || calls[0].Err != nil || !janzeer.IsRPCCode(calls[1].Err, janzeer.RPCNotFound) || bal != "12.50000000" {
		t.Fatalf("batch: %d %v %v %s", nonce, calls[0].Err, calls[1].Err, bal)
	}
}

type fakeWaiter struct{ views []*janzeer.TxView }

func (f *fakeWaiter) WaitForFinalityOnce(context.Context, string, time.Duration) (*janzeer.TxView, error) {
	if len(f.views) == 0 {
		return nil, &janzeer.RPCError{Code: janzeer.RPCNotFound, Message: "unknown"}
	}
	v := f.views[0]
	f.views = f.views[1:]
	return v, nil
}

func TestWaitForFinality(t *testing.T) {
	h := int64(5)
	w := &fakeWaiter{views: []*janzeer.TxView{{Hash: "ab", Status: "PENDING", TimedOut: true}, {Hash: "ab", Status: "FINAL", BlockHeight: &h}}}
	v, err := rpc.WaitForFinality(context.Background(), w, "ab")
	if err != nil || v.Status != "FINAL" || *v.BlockHeight != 5 {
		t.Fatalf("%v %v", v, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	var ft *janzeer.FinalityTimeoutError
	if _, err := rpc.WaitForFinality(ctx, &fakeWaiter{}, "never"); !errors.As(err, &ft) || ft.Hash != "never" {
		t.Fatalf("timeout: %v", err)
	}
}

// A WebSocket server that answers like the node, pushes one notification per subscription, and can drop the socket.
func TestWebSocketCallsSubscriptionsAndReconnect(t *testing.T) {
	if rpc.NormalizeWSURL("https://h/api/v1") != "wss://h/rpc/ws" || rpc.NormalizeWSURL("ws://h/rpc") != "ws://h/rpc/ws" {
		t.Fatal("URL normalization")
	}
	var mu sync.Mutex
	conns := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		mu.Lock()
		conns++
		first := conns == 1
		mu.Unlock()
		ctx := r.Context()
		for {
			_, data, err := c.Read(ctx)
			if err != nil {
				return
			}
			var q req
			_ = json.Unmarshal(data, &q)
			_ = c.Write(ctx, websocket.MessageText, []byte(answer(q)))
			if q.Method == "janzeer_subscribe" {
				_ = c.Write(ctx, websocket.MessageText, []byte(`{"jsonrpc":"2.0","method":"janzeer_subscription","params":{"subscription":"00112233aabbccdd","kind":"addressActivity","result":{"blockHeight":7,"blockHash":"cd","transaction":{"hash":"ab","status":"FINAL","amount":1.25000000}}}}`))
				if first { // drop the first connection right after its notification: the client must come back
					c.Close(websocket.StatusGoingAway, "bye")
					return
				}
			}
		}
	}))
	defer s.Close()

	events := make(chan string, 16)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	ws, err := rpc.Dial(ctx, s.URL, &rpc.WSOptions{ReconnectDelay: 20 * time.Millisecond, OnEvent: func(e string, _ error) { events <- e }})
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	if tip, err := ws.GetTip(ctx); err != nil || tip.Height != 42 {
		t.Fatalf("call over ws: %v", err)
	}
	got := make(chan *rpc.AddressActivity, 4)
	sub, err := ws.SubscribeAddressActivity(ctx, []string{addr}, func(ev *rpc.AddressActivity) { got <- ev })
	if err != nil || sub.ID() != "00112233aabbccdd" {
		t.Fatalf("subscribe: %v", err)
	}
	for i := 0; i < 2; i++ { // one notification before the drop, one after the automatic resubscription
		select {
		case ev := <-got:
			if ev.BlockHeight != 7 || ev.Transaction.Hash != "ab" || *ev.Transaction.Amount != "1.25000000" {
				t.Fatalf("event %+v", ev)
			}
		case <-ctx.Done():
			t.Fatalf("notification %d did not arrive", i+1)
		}
	}
	resubscribed := false
	for !resubscribed {
		select {
		case e := <-events:
			resubscribed = e == "resubscribed"
		case <-ctx.Done():
			t.Fatal("no resubscribed event")
		}
	}
	if err := sub.Unsubscribe(ctx); err != nil {
		t.Fatalf("unsubscribe: %v", err)
	}
	if _, err := ws.SubscribeAddressActivity(ctx, nil, func(*rpc.AddressActivity) {}); err == nil {
		t.Fatal("an empty address list must be refused")
	}
}
