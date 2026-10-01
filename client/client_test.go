package client_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	janzeer "github.com/janzeerorg/janzeer-sdk-go"
	"github.com/janzeerorg/janzeer-sdk-go/client"
)

const addr = "0x74d2bedc03ae5deb6fc63fbf7bb87e86f034b274"

func env(payload string) string { return `{"timestamp":1,"version":"1.1.0","payload":` + payload + `}` }

func server(t *testing.T, h http.HandlerFunc) *client.Client {
	t.Helper()
	s := httptest.NewServer(h)
	t.Cleanup(s.Close)
	return client.New(s.URL)
}

func TestURLAndReads(t *testing.T) {
	if client.New("http://h:7019").BaseURL != "http://h:7019/api/v1/" || client.New("http://h:7019/api/v1/").BaseURL != "http://h:7019/api/v1/" {
		t.Fatal("URL normalization")
	}
	c := server(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/nonce"):
			io.WriteString(w, env("7"))
		case strings.Contains(r.URL.Path, "/wallets/"):
			io.WriteString(w, env("77499900.00200001"))
		case strings.HasSuffix(r.URL.Path, "/info"):
			io.WriteString(w, env(`{"networkId":"janzeer","version":"0.1.0","nodeKey":"02ab","faucet":false}`))
		case r.URL.Path == "/api/v1/transactions/transfers":
			if q := r.URL.Query(); q.Get("address") != addr || q.Get("size") != "5" || q.Get("unconfirmed") != "true" {
				t.Errorf("query: %s", r.URL.RawQuery)
			}
			io.WriteString(w, env(`{"total":1,"list":[{"hash":"ab","amount":0.10000000,"fee":1E-2,"blockHash":null,"extra":9}],"page":0,"pageSize":5,"totalPages":1}`))
		default:
			w.WriteHeader(404)
			io.WriteString(w, env(`{"status":404,"message":"not found"}`))
		}
	})
	ctx := context.Background()
	if b, err := c.Balance(ctx, addr); err != nil || b != "77499900.00200001" { // a float64 would print …00200002
		t.Fatalf("balance %q %v", b, err)
	}
	if n, err := c.Nonce(ctx, addr); err != nil || n != 7 || c.LastEnvelope().Version != "1.1.0" {
		t.Fatalf("nonce %d %v", n, err)
	}
	info, err := c.Info(ctx)
	if err != nil || info.NetworkID != "janzeer" || info.NodeKey != "02ab" {
		t.Fatalf("info %+v %v", info, err)
	}
	page, err := c.List(ctx, client.Transfers, client.ListOptions{Address: addr, Size: 5, Unconfirmed: true})
	if err != nil || page.Total != 1 || *page.List[0].Amount != "0.10000000" || page.List[0].Fee != "0.01" || page.List[0].Final() || !strings.Contains(string(page.List[0].Raw), `"extra":9`) {
		t.Fatalf("page %+v %v", page, err)
	}
	if tx, err := c.Transaction(ctx, client.Transfers, "zz"); tx != nil || err != nil {
		t.Fatalf("an unknown hash is nil, nil: %v %v", tx, err)
	}
	if r, err := c.Receipt(ctx, "zz"); r != nil || err != nil {
		t.Fatal("an unknown receipt is nil, nil")
	}
}

func TestUnknownAddressDefaults(t *testing.T) {
	c := server(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		io.WriteString(w, env(`{"message":"x"}`))
	})
	if b, err := c.Balance(context.Background(), addr); err != nil || b != "0" {
		t.Fatalf("balance %q %v", b, err)
	}
	if n, err := c.Nonce(context.Background(), addr); err != nil || n != 0 {
		t.Fatalf("nonce %d %v", n, err)
	}
	var nf *janzeer.NotFoundError
	if _, err := c.Info(context.Background()); !errors.As(err, &nf) {
		t.Fatalf("info on 404: %v", err)
	}
}

func TestSubmitBodyAndRejection(t *testing.T) {
	acct, _ := janzeer.NewRandomAccount()
	utx, _ := janzeer.NewTransfer(janzeer.Transfer{Common: janzeer.Common{From: acct.Address(), Nonce: 4, Timestamp: 1700000000000}, To: addr, Amount: "1.25"})
	tx, _ := acct.SignTx(utx)
	var reject atomic.Bool
	c := server(t, func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		dec := json.NewDecoder(r.Body)
		dec.UseNumber()
		_ = dec.Decode(&b)
		if b["amount"] != "1.25" || b["fee"] != "0.01" || b["nonce"].(json.Number).String() != "4" || b["hash"] != tx.Hash || r.URL.Path != "/api/v1/transactions/transfers" {
			t.Errorf("body %v path %s", b, r.URL.Path)
		}
		if _, has := b["data"]; has {
			t.Error("an empty memo must be omitted")
		}
		if reject.Load() {
			w.WriteHeader(400)
			io.WriteString(w, env(`{"status":400,"message":"Invalid nonce for `+addr+`: expected 5, got 4","type":"INVALID_NONCE"}`))
			return
		}
		w.WriteHeader(201)
		io.WriteString(w, env(`{"hash":"`+tx.Hash+`","fee":0.01}`))
	})
	if echo, err := c.Submit(context.Background(), tx); err != nil || echo.Hash != tx.Hash {
		t.Fatalf("submit: %v", err)
	}
	reject.Store(true)
	var nm *janzeer.NonceMismatchError
	if _, err := c.Submit(context.Background(), tx); !errors.As(err, &nm) || nm.Expected != 5 {
		t.Fatalf("rejection: %v", err)
	}
}

func TestRateLimitRetriesOnce(t *testing.T) {
	var calls atomic.Int32
	c := server(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(429)
			io.WriteString(w, `{"status":429,"message":"slow down"}`)
			return
		}
		io.WriteString(w, env("5"))
	})
	if n, err := c.Nonce(context.Background(), addr); err != nil || n != 5 || calls.Load() != 2 {
		t.Fatalf("retry: %d %v calls=%d", n, err, calls.Load())
	}
	calls.Store(0)
	c.NoRetryOnRateLimit = true
	var rl *janzeer.RateLimitedError
	if _, err := c.Nonce(context.Background(), addr); !errors.As(err, &rl) {
		t.Fatalf("no retry: %v", err)
	}
}

func TestNetworkErrorAndFinalityPolling(t *testing.T) {
	var ne *janzeer.NetworkError
	if _, err := client.New("http://127.0.0.1:1").Info(context.Background()); !errors.As(err, &ne) {
		t.Fatalf("unreachable: %v", err)
	}
	var n atomic.Int32
	c := server(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/transactions/transfers/ab" {
			w.WriteHeader(404)
			io.WriteString(w, env(`{"message":"x"}`))
			return
		}
		switch n.Add(1) {
		case 1:
			w.WriteHeader(404)
			io.WriteString(w, env(`{"message":"x"}`))
		case 2:
			io.WriteString(w, env(`{"hash":"ab","blockHash":null}`))
		default:
			io.WriteString(w, env(`{"hash":"ab","blockHash":"cd"}`))
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if tx, err := c.WaitForFinality(ctx, "ab", 5*time.Millisecond); err != nil || !tx.Final() {
		t.Fatalf("finality: %v", err)
	}
	short, cancel2 := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel2()
	var ft *janzeer.FinalityTimeoutError
	if _, err := c.WaitForFinality(short, "never", 5*time.Millisecond); !errors.As(err, &ft) {
		t.Fatalf("timeout: %v", err)
	}
}
