// Package janzeer is the official Go SDK of the Janzeer blockchain: keys and addresses, exact amounts, transaction
// builders and signing. The network clients live in the sub-packages client (REST), rpc (JSON-RPC over HTTP and
// WebSocket) and vault (a secret at rest).
//
// Non-custodial by construction: keys never leave your process; the node only verifies.
//
//	me, _ := janzeer.AccountFromMnemonic("abandon … about", "")
//	c := client.New("https://onion.janzeer.org")
//	nonce, _ := c.Nonce(ctx, me.Address())
//	tx, _ := janzeer.NewTransfer(janzeer.Transfer{From: me.Address(), To: "0x…", Amount: "1.25", Nonce: nonce})
//	signed, _ := me.SignTx(tx)
//	_, err := c.Submit(ctx, signed)
package janzeer

// Chain constants. Every value mirrors the node and is pinned by the conformance vectors or the e2e flow.
const (
	// NetworkID is consensus.network-id, bound into every signed transaction (cross-network replay protection).
	NetworkID = "janzeer"
	// Decimals of the native coin: amounts are scaled x10^8 into an int64 on the wire.
	Decimals = 8
	// Ticker is the display ticker of the native coin.
	Ticker = "JNZ"
	// MinFee is the minimum fee of every transaction, in JNZ.
	MinFee = "0.01"
	// MinTransfer is the minimum transfer amount in JNZ unless the transfer carries a memo.
	MinTransfer = "0.1"
	// MaxMemoBytes is the maximum memo length in UTF-8 bytes.
	MaxMemoBytes = 256
	// ValidatorFee is the exact fee of a validator registration.
	ValidatorFee = "3"
	// ValidatorDeposit is the exact, NON-REFUNDABLE deposit of a validator registration.
	ValidatorDeposit = "2000"
	// TokenCreateFee is the exact fee of a token CREATE; every other token operation pays MinFee.
	TokenCreateFee = "5"
)

// The node, API and wire-protocol versions this SDK release was built and tested against.
const (
	SpecNode     = "0.1.0"
	SpecAPI      = "1.1.0"
	SpecProtocol = "3.4.0"
	SpecVectors  = 2
)

// TokenOp is a JZT-1 token operation code (the op byte).
type TokenOp byte

// Token operations.
const (
	TokenCreate TokenOp = iota
	TokenMint
	TokenBurn
	TokenSetCap
	TokenTransfer
)

// String is the operation name as the node reports it.
func (o TokenOp) String() string {
	names := [...]string{"CREATE", "MINT", "BURN", "SETCAP", "TRANSFER"}
	if int(o) < len(names) {
		return names[o]
	}
	return "UNKNOWN"
}

// Server-side limits (informational; the node enforces them).
const (
	LimitPageSize          = 100
	LimitBlockRange        = 100
	LimitWaitForFinalityMs = 60_000
	LimitSubscriptions     = 16
	LimitWatchedAddresses  = 1000
	LimitRPCBatch          = 50
	LimitRPCBodyBytes      = 524_288
)
