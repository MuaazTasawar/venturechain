package wallet

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/MuaazTasawar/venturechain/pkg/crypto"
)

// StoredKey is the on-disk representation of a wallet identity. In this
// devnet setup the private key is stored in plaintext JSON for simplicity -
// production would encrypt this at rest with a passphrase-derived key.
type StoredKey struct {
	Label      string `json:"label"`
	Address    string `json:"address"`
	PublicKey  string `json:"public_key"`
	PrivateKey string `json:"private_key"`
}

// GenerateAndSave creates a new key pair and writes it to disk at path as
// JSON. Returns the generated key so the caller can print it immediately.
func GenerateAndSave(label, path string) (*StoredKey, error) {
	kp, err := crypto.GenerateKeyPair()
	if err != nil {
		return nil, fmt.Errorf("key generation failed: %w", err)
	}

	stored := &StoredKey{
		Label:      label,
		Address:    kp.Address,
		PublicKey:  kp.PublicKeyHex,
		PrivateKey: kp.PrivateKeyHex,
	}

	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return nil, fmt.Errorf("failed to write key file: %w", err)
	}
	return stored, nil
}

// Load reads a previously saved wallet identity from disk.
func Load(path string) (*StoredKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read key file: %w", err)
	}
	var stored StoredKey
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, fmt.Errorf("failed to parse key file: %w", err)
	}
	return &stored, nil
}