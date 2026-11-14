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

// Page is a generic paged result.
type Page[T any] struct {
	Items []T   `json:"items"`
	Total int64 `json:"total"`
}

// TokenView represents token metadata.
type TokenView struct {
	Address     string `json:"address"`
	Symbol      string `json:"symbol"`
	Name        string `json:"name"`
	Decimals    int    `json:"decimals"`
	TotalSupply Amount `json:"totalSupply"`
	Owner       string `json:"owner"`
}

// TokenBalanceView represents a balance of a specific token.
type TokenBalanceView struct {
	Token   string `json:"token"`
	Balance Amount `json:"balance"`
}

// TokenHolderView represents an account holding a token.
type TokenHolderView struct {
	Address string `json:"address"`
	Balance Amount `json:"balance"`
}

// GenesisView represents the chain genesis information.
type GenesisView struct {
	GenesisHash string `json:"genesisHash"`
	GenesisTime int64  `json:"genesisTime"`
	ChainID     string `json:"chainId"`
}

// NodeInfo is GET info.
type NodeInfo struct {
	NodeKey     string `json:"nodeKey"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	NetworkID   string `json:"networkId"`
	GenesisHash string `json:"genesisHash"`
}
