package crypto

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
)

// KeyPair holds a generated wallet identity.
type KeyPair struct {
	PrivateKeyHex string
	PublicKeyHex  string
	Address       string
}

// GenerateKeyPair creates a new secp256k1 private/public key pair and derives
// a VentureChain address from the public key.
func GenerateKeyPair() (*KeyPair, error) {
	priv, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		return nil, err
	}
	pub := priv.PubKey()

	privHex := hex.EncodeToString(priv.Serialize())
	pubHex := hex.EncodeToString(pub.SerializeCompressed())
	address := AddressFromPubKeyHex(pubHex)

	return &KeyPair{
		PrivateKeyHex: privHex,
		PublicKeyHex:  pubHex,
		Address:       address,
	}, nil
}

// AddressFromPubKeyHex derives a human-readable VentureChain address from a
// hex-encoded compressed public key: VTC + first 40 hex chars of sha256(pubkey).
func AddressFromPubKeyHex(pubKeyHex string) string {
	pubBytes, err := hex.DecodeString(pubKeyHex)
	if err != nil {
		return ""
	}
	hash := sha256.Sum256(pubBytes)
	return "VTC" + hex.EncodeToString(hash[:])[:40]
}

// Sign signs a message hash with a hex-encoded private key and returns a
// hex-encoded DER signature.
func Sign(privKeyHex string, hash []byte) (string, error) {
	privBytes, err := hex.DecodeString(privKeyHex)
	if err != nil {
		return "", err
	}
	if len(privBytes) != 32 {
		return "", errors.New("invalid private key length")
	}
	priv := secp256k1.PrivKeyFromBytes(privBytes)
	sig := ecdsa.Sign(priv, hash)
	return hex.EncodeToString(sig.Serialize()), nil
}

// Verify checks a hex-encoded DER signature against a message hash and a
// hex-encoded compressed public key.
func Verify(pubKeyHex string, hash []byte, sigHex string) (bool, error) {
	pubBytes, err := hex.DecodeString(pubKeyHex)
	if err != nil {
		return false, err
	}
	pub, err := secp256k1.ParsePubKey(pubBytes)
	if err != nil {
		return false, err
	}
	sigBytes, err := hex.DecodeString(sigHex)
	if err != nil {
		return false, err
	}
	sig, err := ecdsa.ParseDERSignature(sigBytes)
	if err != nil {
		return false, err
	}
	return sig.Verify(hash, pub), nil
}

// HashBytes returns the sha256 hash of arbitrary data.
func HashBytes(data []byte) []byte {
	h := sha256.Sum256(data)
	return h[:]
}