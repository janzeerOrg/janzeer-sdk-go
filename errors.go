package janzeer

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Error types. Use errors.As to branch on the type, never on message text. A transaction the node refuses is a
// *TxRejectedError whichever transport carried it (REST 400 with a type, or JSON-RPC -32001); a nonce problem is
// additionally a *NonceMismatchError (errors.As finds both).

// NetworkError: the node could not be reached, the response was not JSON, or the socket died.
type NetworkError struct {
	Message string
	Err     error
}

func (e *NetworkError) Error() string { return e.Message }
func (e *NetworkError) Unwrap() error { return e.Err }

// TxRejectedError: the node refused a transaction at intake (validation or mempool rule).
type TxRejectedError struct {
	Message string
	// Type is the node's exception type (INCORRECT_SIGNATURE, INSUFFICIENT_BALANCE, …) or UNKNOWN.
	Type string
	// Transport is "rest", "rpc" or "ws".
	Transport  string
	HTTPStatus int
	RPCCode    int
}

func (e *TxRejectedError) Error() string { return e.Message }

// NonceMismatchError: the nonce is not the sender's next nonce. Refresh it (client.Nonce) and rebuild.
type NonceMismatchError struct {
	TxRejectedError
	Address string
	// Expected and Got are valid when Known is true (the node's message carried them).
	Expected, Got int64
	Known         bool
}

// Unwrap lets errors.As(err, **TxRejectedError) match a nonce mismatch too.
func (e *NonceMismatchError) Unwrap() error { return &e.TxRejectedError }

// APIError: any other REST error ({status, message, type?} payload).
type APIError struct {
	Status  int
	Message string
	Type    string
	Body    json.RawMessage
}

func (e *APIError) Error() string { return e.Message }

// ValidationError: request-body validation failed (HTTP 400 with a list of messages).
type ValidationError struct {
	APIError
	Messages []string
}

func (e *ValidationError) Unwrap() error { return &e.APIError }

// NotFoundError: HTTP 404.
type NotFoundError struct{ APIError }

func (e *NotFoundError) Unwrap() error { return &e.APIError }

// NotSynchronizedError: the node is still syncing (REST 400 / RPC -32002). Retry shortly.
type NotSynchronizedError struct{ Message, Transport string }

func (e *NotSynchronizedError) Error() string { return e.Message }

// RateLimitedError: HTTP 429 / RPC -32004.
type RateLimitedError struct {
	Message    string
	RetryAfter time.Duration
	Transport  string
}

func (e *RateLimitedError) Error() string { return e.Message }

// RPCError: a JSON-RPC error that is not a transaction rejection. Compare Code with the RPC* constants.
type RPCError struct {
	Code    int
	Message string
	Data    json.RawMessage
}

func (e *RPCError) Error() string { return e.Message }

// JSON-RPC error codes of the node.
const (
	RPCParseError      = -32700
	RPCInvalidRequest  = -32600
	RPCMethodNotFound  = -32601
	RPCInvalidParams   = -32602
	RPCInternal        = -32603
	RPCNotFound        = -32000
	RPCRejected        = -32001
	RPCNotSynchronized = -32002
	RPCLimit           = -32003
	RPCRateLimited     = -32004
)

// IsRPCCode reports whether err is an *RPCError with the given code.
func IsRPCCode(err error, code int) bool {
	var e *RPCError
	return errors.As(err, &e) && e.Code == code
}

// FinalityTimeoutError: the wait deadline passed; Last is the most recent view (possibly nil).
type FinalityTimeoutError struct {
	Hash string
	Last *TxView
}

func (e *FinalityTimeoutError) Error() string {
	status := "unknown"
	if e.Last != nil {
		status = e.Last.Status
	}
	return fmt.Sprintf("janzeer: transaction %s not final within the deadline (last status: %s)", e.Hash, status)
}

var nonceRe = regexp.MustCompile(`Invalid nonce for (0x[0-9a-fA-F]{40}): expected (\d+), got (\d+)`)

func nonceFromMessage(message, transport string, httpStatus, rpcCode int) *NonceMismatchError {
	m := nonceRe.FindStringSubmatch(message)
	if m == nil {
		return nil
	}
	exp, _ := strconv.ParseInt(m[2], 10, 64)
	got, _ := strconv.ParseInt(m[3], 10, 64)
	return &NonceMismatchError{TxRejectedError: TxRejectedError{Message: message, Type: "INVALID_NONCE", Transport: transport, HTTPStatus: httpStatus, RPCCode: rpcCode},
		Address: strings.ToLower(m[1]), Expected: exp, Got: got, Known: true}
}

// RPCErrorFrom maps a JSON-RPC error object to the SDK error types. transport is "rpc" or "ws".
func RPCErrorFrom(code int, message string, data json.RawMessage, transport string) error {
	if message == "" {
		message = fmt.Sprintf("RPC error %d", code)
	}
	switch code {
	case RPCRejected:
		if n := nonceFromMessage(message, transport, 0, code); n != nil {
			return n
		}
		var d struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal(data, &d)
		if d.Type == "INVALID_NONCE" {
			return &NonceMismatchError{TxRejectedError: TxRejectedError{Message: message, Type: d.Type, Transport: transport, RPCCode: code}}
		}
		if d.Type == "" {
			d.Type = "UNKNOWN"
		}
		return &TxRejectedError{Message: message, Type: d.Type, Transport: transport, RPCCode: code}
	case RPCNotSynchronized:
		return &NotSynchronizedError{Message: message, Transport: transport}
	case RPCRateLimited:
		return &RateLimitedError{Message: message, RetryAfter: time.Second, Transport: transport}
	}
	return &RPCError{Code: code, Message: message, Data: data}
}

// RESTErrorFrom maps a non-2xx REST response to the SDK error types. Rule: a body with "payload" -> use it (the
// envelope wraps errors too); otherwise the body itself (the rate limiter answers before the envelope layer).
func RESTErrorFrom(status int, body []byte, retryAfter string) error {
	payload := json.RawMessage(body)
	var env struct {
		Payload json.RawMessage `json:"payload"`
	}
	if json.Unmarshal(body, &env) == nil && env.Payload != nil {
		payload = env.Payload
	}
	var obj struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	}
	_ = json.Unmarshal(payload, &obj)
	if status == 429 {
		seconds, err := strconv.ParseFloat(strings.TrimSpace(retryAfter), 64)
		if err != nil {
			seconds = 1
		}
		msg := obj.Message
		if msg == "" {
			msg = "Rate limit exceeded"
		}
		return &RateLimitedError{Message: msg, RetryAfter: time.Duration(seconds * float64(time.Second)), Transport: "rest"}
	}
	var list []json.RawMessage
	if json.Unmarshal(payload, &list) == nil && list != nil {
		msgs := make([]string, 0, len(list))
		for _, item := range list {
			var m struct {
				Message *string `json:"message"`
			}
			if json.Unmarshal(item, &m) == nil && m.Message != nil {
				msgs = append(msgs, *m.Message)
			} else {
				msgs = append(msgs, strings.Trim(string(item), `"`))
			}
		}
		text := strings.Join(msgs, "; ")
		if text == "" {
			text = "validation failed"
		}
		return &ValidationError{APIError: APIError{Status: 400, Message: text, Body: payload}, Messages: msgs}
	}
	message := obj.Message
	if message == "" {
		message = fmt.Sprintf("Request failed (%d)", status)
	}
	api := APIError{Status: status, Message: message, Type: obj.Type, Body: payload}
	if status == 404 {
		return &NotFoundError{APIError: api}
	}
	if status == 400 {
		if message == "Blockchain is synchronizing" {
			return &NotSynchronizedError{Message: message, Transport: "rest"}
		}
		if n := nonceFromMessage(message, "rest", status, 0); n != nil {
			return n
		}
		if obj.Type == "INVALID_NONCE" {
			return &NonceMismatchError{TxRejectedError: TxRejectedError{Message: message, Type: obj.Type, Transport: "rest", HTTPStatus: status}}
		}
		if obj.Type != "" {
			return &TxRejectedError{Message: message, Type: obj.Type, Transport: "rest", HTTPStatus: status}
		}
	}
	return &api
}
