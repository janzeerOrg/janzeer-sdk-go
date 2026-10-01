package janzeer

import (
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"
)

// Transactions: the signed-bytes layouts, the unsigned / signed objects and the builders. Every multi-byte integer
// is big-endian; strings and addresses are their UTF-8 bytes (an address is signed as text, not decoded). The
// builders validate what the node validates at intake, so a mistake fails locally with a clear message.

// TxType names a client-submittable transaction type.
type TxType string

// Transaction types.
const (
	TxTransfer          TxType = "transfer"
	TxToken             TxType = "token"
	TxRegisterValidator TxType = "registerValidator"
	TxExitValidator     TxType = "exitValidator"
)

var txRoutes = map[TxType][2]string{
	TxTransfer:          {"transactions/transfers", "janzeer_sendTransfer"},
	TxToken:             {"transactions/tokens", "janzeer_sendToken"},
	TxRegisterValidator: {"transactions/validators", "janzeer_sendRegisterValidator"},
	TxExitValidator:     {"transactions/exit-validators", "janzeer_sendExitValidator"},
}

// UnsignedTx is a transaction that is fully specified but not yet signed. Sign it with Account.SignTx.
type UnsignedTx struct {
	Type          TxType
	NetworkID     string
	Timestamp     int64 // ms since epoch
	Fee           string
	Nonce         int64
	SenderAddress string
	// Fields are the type-specific fields exactly as they go into the request body.
	Fields map[string]any
	// Payload is the type-specific part of the signed bytes.
	Payload []byte
}

// Preimage returns the exact bytes that are hashed and signed:
// networkId || timestamp(8) || scaled(fee)(8) || nonce(8) || senderAddress || payload.
func (t *UnsignedTx) Preimage() []byte {
	fee, _ := ToScaled(t.Fee) // validated by the builder
	if fee == nil {
		fee = new(big.Int)
	}
	out := make([]byte, 0, 96+len(t.Payload))
	out = append(out, t.NetworkID...)
	out = append(out, i64be(t.Timestamp)...)
	out = append(out, i64be(fee.Int64())...)
	out = append(out, i64be(t.Nonce)...)
	out = append(out, t.SenderAddress...)
	return append(out, t.Payload...)
}

// Hash is double-SHA256 of the preimage, lowercase hex: the transaction id.
func (t *UnsignedTx) Hash() string { return HashPreimage(t.Preimage()) }

// WithSignature attaches a signature made elsewhere (hardware wallet, KMS). Prefer Account.SignTx.
func (t *UnsignedTx) WithSignature(signatureBase64, senderPublicKeyHex string) *SignedTx {
	return &SignedTx{Unsigned: t, Hash: t.Hash(), Signature: signatureBase64, SenderPublicKey: senderPublicKeyHex}
}

// SignedTx is a signed transaction ready for client.Submit or rpc.Send.
type SignedTx struct {
	Unsigned *UnsignedTx
	Hash     string
	// Signature is the Base64 DER ECDSA signature over Hash.
	Signature string
	// SenderPublicKey is the compressed secp256k1 public key, hex.
	SenderPublicKey string
}

// RESTPath is the path (relative to the API base) that accepts this transaction.
func (s *SignedTx) RESTPath() string { return txRoutes[s.Unsigned.Type][0] }

// RPCMethod is the JSON-RPC method that accepts this transaction.
func (s *SignedTx) RPCMethod() string { return txRoutes[s.Unsigned.Type][1] }

// Body is the JSON body the node's REST endpoints and janzeer_send* methods accept. Amounts are strings.
func (s *SignedTx) Body() map[string]any {
	u := s.Unsigned
	body := map[string]any{
		"timestamp": u.Timestamp, "fee": u.Fee, "nonce": u.Nonce, "hash": s.Hash,
		"senderAddress": u.SenderAddress, "senderPublicKey": s.SenderPublicKey, "senderSignature": s.Signature,
	}
	for k, v := range u.Fields {
		switch x := v.(type) {
		case nil:
			continue
		case string:
			if x == "" && (u.Type == TxToken || k == "data") {
				continue
			}
		}
		body[k] = v
	}
	return body
}

// Common holds the fields every builder takes. Nonce comes from the node (client.Nonce).
type Common struct {
	// From is the sender address (an EIP-55 address is normalized).
	From string
	// Nonce is the sender's next nonce (committed nonce + pending count).
	Nonce int64
	// Fee defaults to the type's required or minimum fee.
	Fee string
	// Timestamp in ms since epoch; defaults to now.
	Timestamp int64
	// NetworkID defaults to NetworkID ("janzeer"); pass the node's id when working on a testnet.
	NetworkID string
}

func (c Common) base(t TxType, defaultFee string) (*UnsignedTx, error) {
	fee := c.Fee
	if fee == "" {
		fee = defaultFee
	}
	fee, err := NormalizeAmount(fee)
	if err != nil || !IsValidAmount(fee) {
		return nil, fmt.Errorf("janzeer: fee must be a decimal >= %s, got %q", MinFee, c.Fee)
	}
	if cmp, _ := CompareAmounts(fee, MinFee); cmp < 0 {
		return nil, fmt.Errorf("janzeer: fee must be a decimal >= %s, got %s", MinFee, fee)
	}
	if c.Nonce < 0 {
		return nil, fmt.Errorf("janzeer: nonce must be non-negative, got %d", c.Nonce)
	}
	sender, err := NormalizeAddress(c.From)
	if err != nil {
		return nil, err
	}
	ts := c.Timestamp
	if ts == 0 {
		ts = time.Now().UnixMilli()
	}
	nid := c.NetworkID
	if nid == "" {
		nid = NetworkID
	}
	return &UnsignedTx{Type: t, NetworkID: nid, Timestamp: ts, Fee: fee, Nonce: c.Nonce, SenderAddress: sender}, nil
}

func scaledBytes(amount string) []byte {
	v, _ := ToScaled(amount)
	return i64be(v.Int64())
}

// Transfer describes a native-coin transfer.
type Transfer struct {
	Common
	To     string
	Amount string
	// Data is an optional memo, at most 256 UTF-8 bytes; with a memo the amount may be below MinTransfer (even "0").
	Data string
}

// NewTransfer builds a native-coin transfer. The fee defaults to MinFee.
func NewTransfer(o Transfer) (*UnsignedTx, error) {
	tx, err := o.Common.base(TxTransfer, MinFee)
	if err != nil {
		return nil, err
	}
	amount, err := NormalizeAmount(o.Amount)
	if err != nil || !IsValidAmount(amount) || strings.HasPrefix(amount, "-") {
		return nil, fmt.Errorf("janzeer: amount must be a decimal with <= 8 fractional digits, got %q", o.Amount)
	}
	if len(o.Data) > MaxMemoBytes {
		return nil, fmt.Errorf("janzeer: memo exceeds %d UTF-8 bytes", MaxMemoBytes)
	}
	if cmp, _ := CompareAmounts(amount, MinTransfer); o.Data == "" && cmp < 0 {
		return nil, fmt.Errorf("janzeer: amount must be >= %s unless the transfer carries a memo", MinTransfer)
	}
	to, err := NormalizeAddress(o.To)
	if err != nil {
		return nil, err
	}
	tx.Fields = map[string]any{"amount": amount, "recipientAddress": to, "data": o.Data}
	tx.Payload = append(append(scaledBytes(amount), to...), o.Data...)
	return tx, nil
}

var (
	keyRe     = regexp.MustCompile(`^0[23][0-9a-fA-F]{64}$`)
	tokenIDRe = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)
	unitsRe   = regexp.MustCompile(`^\d+$`)
)

func checkKey(key string) (string, error) {
	if !keyRe.MatchString(key) {
		return "", errors.New("janzeer: validator key must be a compressed secp256k1 public key (66 hex chars)")
	}
	return strings.ToLower(key), nil
}

// RegisterValidator describes a validator registration.
type RegisterValidator struct {
	Common
	// ValidatorKey is the NODE's public key (GET info -> nodeKey), never the wallet's own key.
	ValidatorKey string
	// Amount must equal ValidatorDeposit; it defaults to it.
	Amount string
}

// NewRegisterValidator builds a validator registration: the exact ValidatorFee and the NON-REFUNDABLE ValidatorDeposit.
func NewRegisterValidator(o RegisterValidator) (*UnsignedTx, error) {
	tx, err := o.Common.base(TxRegisterValidator, ValidatorFee)
	if err != nil {
		return nil, err
	}
	if cmp, _ := CompareAmounts(tx.Fee, ValidatorFee); cmp != 0 {
		return nil, fmt.Errorf("janzeer: validator registration fee must be exactly %s", ValidatorFee)
	}
	amount := o.Amount
	if amount == "" {
		amount = ValidatorDeposit
	}
	if cmp, err := CompareAmounts(amount, ValidatorDeposit); err != nil || cmp != 0 {
		return nil, fmt.Errorf("janzeer: validator deposit must be exactly %s", ValidatorDeposit)
	}
	amount, _ = NormalizeAmount(amount)
	key, err := checkKey(o.ValidatorKey)
	if err != nil {
		return nil, err
	}
	tx.Fields = map[string]any{"amount": amount, "validatorKey": key}
	tx.Payload = append(scaledBytes(amount), key...)
	return tx, nil
}

// ExitValidator describes a validator exit.
type ExitValidator struct {
	Common
	ValidatorKey string
}

// NewExitValidator builds a graceful validator exit (removed at the next epoch; the deposit stays locked).
func NewExitValidator(o ExitValidator) (*UnsignedTx, error) {
	tx, err := o.Common.base(TxExitValidator, MinFee)
	if err != nil {
		return nil, err
	}
	key, err := checkKey(o.ValidatorKey)
	if err != nil {
		return nil, err
	}
	tx.Fields = map[string]any{"validatorKey": key}
	tx.Payload = []byte(key)
	return tx, nil
}

// Token describes a JZT-1 token operation. Token amounts (Cap, Amount) are integer base units as digit strings,
// never scaled; Decimals is display-only. Which fields are used depends on the operation.
type Token struct {
	Common
	TokenID   string // every operation except create
	Symbol    string // create
	Name      string // create
	Decimals  int    // create, 0-18
	Cap       string // create (optional), set-cap
	Amount    string // create (optional initial mint), mint, burn, transfer
	Recipient string // mint (optional, defaults to the sender), transfer
}

func (o Token) build(op TokenOp, defaultFee string) (*UnsignedTx, error) {
	tx, err := o.Common.base(TxToken, defaultFee)
	if err != nil {
		return nil, err
	}
	for name, v := range map[string]string{"cap": o.Cap, "amount": o.Amount} {
		if v != "" && !unitsRe.MatchString(v) {
			return nil, fmt.Errorf("janzeer: %s must be a non-negative integer of base units, got %q", name, v)
		}
	}
	tokenID := strings.ToLower(o.TokenID)
	if op != TokenCreate && !tokenIDRe.MatchString(tokenID) {
		return nil, errors.New("janzeer: token id must be a 64-hex transaction hash")
	}
	recipient := o.Recipient
	if recipient != "" {
		if recipient, err = NormalizeAddress(recipient); err != nil {
			return nil, err
		}
	}
	fields := map[string]any{"op": int(op), "tokenId": tokenID, "symbol": o.Symbol, "name": o.Name, "decimals": o.Decimals, "recipient": recipient}
	fields["cap"], fields["amount"] = nilIfEmpty(o.Cap), nilIfEmpty(o.Amount)
	tx.Fields = fields
	p := []byte{byte(op)}
	p = append(p, lenPrefixed(tokenID)...)
	p = append(p, lenPrefixed(o.Symbol)...)
	p = append(p, lenPrefixed(o.Name)...)
	p = append(p, ser32(uint32(o.Decimals))...)
	p = append(p, lenPrefixed(o.Cap)...)
	p = append(p, lenPrefixed(o.Amount)...)
	tx.Payload = append(p, lenPrefixed(recipient)...)
	return tx, nil
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// NewTokenCreate builds a token CREATE; the new token id is the transaction hash. The fee is exactly TokenCreateFee.
func NewTokenCreate(o Token) (*UnsignedTx, error) {
	o.Symbol, o.Name, o.TokenID, o.Recipient = strings.TrimSpace(o.Symbol), strings.TrimSpace(o.Name), "", ""
	if o.Symbol == "" || o.Name == "" {
		return nil, errors.New("janzeer: symbol and name are required")
	}
	if o.Decimals < 0 || o.Decimals > 18 {
		return nil, errors.New("janzeer: decimals must be 0-18")
	}
	tx, err := o.build(TokenCreate, TokenCreateFee)
	if err != nil {
		return nil, err
	}
	if cmp, _ := CompareAmounts(tx.Fee, TokenCreateFee); cmp != 0 {
		return nil, fmt.Errorf("janzeer: token create fee must be exactly %s", TokenCreateFee)
	}
	return tx, nil
}

func (o Token) only(op TokenOp, needAmount, needCap, needRecipient bool) (*UnsignedTx, error) {
	if needAmount && o.Amount == "" {
		return nil, errors.New("janzeer: amount is required")
	}
	if needCap && o.Cap == "" {
		return nil, errors.New("janzeer: cap is required")
	}
	if needRecipient && o.Recipient == "" {
		return nil, errors.New("janzeer: recipient is required")
	}
	o.Symbol, o.Name, o.Decimals = "", "", 0
	return o.build(op, MinFee)
}

// NewTokenMint builds a MINT (issuer only); Recipient defaults to the sender.
func NewTokenMint(o Token) (*UnsignedTx, error) {
	o.Cap = ""
	return o.only(TokenMint, true, false, false)
}

// NewTokenBurn builds a BURN (issuer only).
func NewTokenBurn(o Token) (*UnsignedTx, error) {
	o.Cap, o.Recipient = "", ""
	return o.only(TokenBurn, true, false, false)
}

// NewTokenSetCap builds a SETCAP (issuer only).
func NewTokenSetCap(o Token) (*UnsignedTx, error) {
	o.Amount, o.Recipient = "", ""
	return o.only(TokenSetCap, false, true, false)
}

// NewTokenTransfer builds a token TRANSFER.
func NewTokenTransfer(o Token) (*UnsignedTx, error) {
	o.Cap = ""
	return o.only(TokenTransfer, true, false, true)
}
