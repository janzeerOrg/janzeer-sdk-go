package vault_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/janzeerorg/janzeer-sdk-go/internal/vectors"
	"github.com/janzeerorg/janzeer-sdk-go/vault"
)

func TestFixtureAndRoundTrip(t *testing.T) {
	var f struct {
		Password, WrongPassword, Secret string
		Blob                            vault.Blob
	}
	if err := json.Unmarshal(vectors.VaultFixture, &f); err != nil {
		t.Fatal(err)
	}
	got, err := vault.Decrypt(&f.Blob, f.Password)
	if err != nil || got != f.Secret {
		t.Fatalf("fixture: %q %v", got, err)
	}
	if _, err := vault.Decrypt(&f.Blob, f.WrongPassword); !errors.Is(err, vault.ErrVault) {
		t.Fatalf("wrong password must fail with ErrVault, got %v", err)
	}
	blob, err := vault.Encrypt("secret words", "pw")
	if err != nil {
		t.Fatal(err)
	}
	if s, err := vault.Decrypt(blob, "pw"); err != nil || s != "secret words" {
		t.Fatal("round trip", err)
	}
	blob.CT = blob.CT[:len(blob.CT)-4] + "AAA="
	if _, err := vault.Decrypt(blob, "pw"); !errors.Is(err, vault.ErrVault) {
		t.Fatal("a tampered blob must fail")
	}
	if _, err := vault.Decrypt(&vault.Blob{V: 2}, "pw"); !errors.Is(err, vault.ErrVault) {
		t.Fatal("an unknown version must fail")
	}
}
