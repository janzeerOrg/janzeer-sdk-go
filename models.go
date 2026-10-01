package janzeer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
)

// Amount is a decimal number exactly as the node wrote it ("1.25000000", "77499900.002"). The node serializes coin
// amounts as bare JSON numbers; decoding them into a float64 would round. Amount keeps the source text.
type Amount string

// UnmarshalJSON accepts a JSON number or a JSON string and keeps its exact text (an exponent is expanded).
func (a *Amount) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if bytes.Equal(b, []byte("null")) {
		*a = ""
		return nil
	}
	s := strings.Trim(string(b), `"`)
	if strings.ContainsAny(s, "eE") {
		f, ok := new(big.Float).SetPrec(256).SetString(s)
		if !ok {
			return fmt.Errorf("janzeer: not a number: %s", s)
		}
		s = strings.TrimRight(strings.TrimRight(f.Text('f', 18), "0"), ".")
	}
	if _, err := NormalizeAmount(s); err != nil {
		return err
	}
	*a = Amount(s)
	return nil
}

// MarshalJSON writes the amount as a JSON string.
func (a Amount) MarshalJSON() ([]byte, error) { return json.Marshal(string(a)) }

// String returns the decimal text.
func (a Amount) String() string { return string(a) }

// Scaled returns the amount in base units (x10^8).
func (a Amount) Scaled() (*big.Int, error) { return ToScaled(string(a)) }

// NodeInfo is GET info.
type NodeInfo struct {
	NodeKey         string `json:"nodeKey"`
	Host            string `json:"host"`
	Port            int    `json:"port"`
	NetworkID       string `json:"networkId"`
	GenesisHash     string `json:"genesisHash"`
	ChainSpecDigest string `json:"chainSpecDigest"`
	Version         string `json:"version"`
	APIVersion      string `json:"apiVersion"`
	ProtocolVersion string `json:"protocolVersion"`
	SyncStatus      string `json:"syncStatus"`
	Faucet          bool   `json:"faucet"`
}

// Page is one page of a REST or RPC list.
type Page[T any] struct {
	Total      int64 `json:"total"`
	List       []T   `json:"list"`
	Page       int   `json:"page"`
	PageSize   int   `json:"pageSize"`
	TotalPages int   `json:"totalPages"`
}

// Transaction is a transaction as the REST lists and lookups return it. Fields that a type does not have are nil.
type Transaction struct {
	Hash             string  `json:"hash"`
	Timestamp        int64   `json:"timestamp"`
	Fee              Amount  `json:"fee"`
	Nonce            *int64  `json:"nonce"`
	SenderAddress    string  `json:"senderAddress"`
	SenderPublicKey  string  `json:"senderPublicKey"`
	SenderSignature  string  `json:"senderSignature"`
	BlockHash        *string `json:"blockHash"`
	Amount           *Amount `json:"amount"`
	RecipientAddress *string `json:"recipientAddress"`
	Data             *string `json:"data"`
	ValidatorKey     *string `json:"validatorKey"`
	TokenID          *string `json:"tokenId"`
	// Raw is the complete JSON object, for fields this struct does not name.
	Raw json.RawMessage `json:"-"`
}

// UnmarshalJSON keeps the raw object next to the named fields.
func (t *Transaction) UnmarshalJSON(b []byte) error {
	type plain Transaction
	if err := json.Unmarshal(b, (*plain)(t)); err != nil {
		return err
	}
	t.Raw = append(t.Raw[:0], b...)
	return nil
}

// Final reports whether the transaction is in a committed block.
func (t *Transaction) Final() bool { return t.BlockHash != nil && *t.BlockHash != "" }

// Receipt is the execution receipt of a committed transaction.
type Receipt struct {
	Successful bool            `json:"successful"`
	Results    json.RawMessage `json:"results"`
}

// TxView is the unified view of any transaction (janzeer_getTransactionByHash, waitForFinality, subscriptions).
type TxView struct {
	Hash string  `json:"hash"`
	Type *string `json:"type"`
	// Status is FINAL, PENDING or UNKNOWN.
	Status           string   `json:"status"`
	Timestamp        *int64   `json:"timestamp"`
	Fee              *Amount  `json:"fee"`
	Nonce            *int64   `json:"nonce"`
	SenderAddress    *string  `json:"senderAddress"`
	SenderPublicKey  *string  `json:"senderPublicKey"`
	SenderSignature  *string  `json:"senderSignature"`
	BlockHash        *string  `json:"blockHash"`
	BlockHeight      *int64   `json:"blockHeight"`
	Receipt          *Receipt `json:"receipt"`
	Amount           *Amount  `json:"amount"`
	RecipientAddress *string  `json:"recipientAddress"`
	Data             *string  `json:"data"`
	ValidatorKey     *string  `json:"validatorKey"`
	Op               *string  `json:"op"`
	TokenID          *string  `json:"tokenId"`
	Symbol           *string  `json:"symbol"`
	Name             *string  `json:"name"`
	Decimals         *int     `json:"decimals"`
	Cap              *Amount  `json:"cap"`
	TokenAmount      *Amount  `json:"tokenAmount"`
	Recipient        *string  `json:"recipient"`
	TimedOut         bool     `json:"timedOut"`
}

// BlockView is a block as the JSON-RPC methods and the newBlocks feed return it.
type BlockView struct {
	Type              string   `json:"type"`
	Height            int64    `json:"height"`
	Hash              string   `json:"hash"`
	PreviousHash      string   `json:"previousHash"`
	Timestamp         int64    `json:"timestamp"`
	Producer          string   `json:"producer"`
	Signature         string   `json:"signature"`
	EpochIndex        *int64   `json:"epochIndex"`
	TransactionsCount int      `json:"transactionsCount"`
	Validators        []string `json:"validators"`
	Transactions      []TxView `json:"transactions"`
}

// AccountView is janzeer_getAccount.
type AccountView struct {
	Address           string `json:"address"`
	Balance           Amount `json:"balance"`
	CommittedBalance  Amount `json:"committedBalance"`
	NextNonce         int64  `json:"nextNonce"`
	CommittedNonce    int64  `json:"committedNonce"`
	PendingCount      int    `json:"pendingCount"`
	IsValidatorWallet bool   `json:"isValidatorWallet"`
}

// SendResult is the answer of the janzeer_send* methods.
type SendResult struct {
	Hash   string `json:"hash"`
	Status string `json:"status"`
}

// Validator is an entry of the REST validator lists.
type Validator struct {
	Address string `json:"address"`
	NodeKey string `json:"nodeKey"`
}

// ValidatorInfo is an entry of the JSON-RPC validator methods.
type ValidatorInfo struct {
	Key           string  `json:"key"`
	WalletAddress *string `json:"walletAddress"`
	RegisteredAt  *int64  `json:"registeredAt"`
	Active        *bool   `json:"active"`
	Banned        *bool   `json:"banned"`
	Exited        *bool   `json:"exited"`
}

// FeeEstimate is janzeer_estimateFee.
type FeeEstimate struct {
	Kind      string `json:"kind"`
	Fee       Amount `json:"fee"`
	Rule      string `json:"rule"`
	FeeMarket bool   `json:"feeMarket"`
}
