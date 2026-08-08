# ── internal/wallet/keys.go ──
New-Item -ItemType Directory -Path internal\wallet -Force | Out-Null
$content = @'
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
'@
[System.IO.File]::WriteAllText("$PWD\internal\wallet\keys.go", $content)

# ── internal/wallet/wallet.go ──
$content = @'
package wallet

import (
	"fmt"

	"github.com/MuaazTasawar/venturechain/internal/blockchain"
	"github.com/MuaazTasawar/venturechain/pkg/crypto"
)

// Wallet wraps a loaded key with convenience methods for building and
// signing transactions against a known nonce.
type Wallet struct {
	Key *StoredKey
}

// New wraps an already-loaded StoredKey in a Wallet.
func New(key *StoredKey) *Wallet {
	return &Wallet{Key: key}
}

// BuildAndSignTransfer creates a signed TRANSFER transaction from this
// wallet to the given recipient address.
func (w *Wallet) BuildAndSignTransfer(to string, amount int64, nonce int64) (*blockchain.Transaction, error) {
	tx := blockchain.NewTransaction(blockchain.TxTransfer, w.Key.Address, to, amount, "", "", nonce)
	if err := tx.Sign(w.Key.PrivateKey, w.Key.PublicKey); err != nil {
		return nil, fmt.Errorf("failed to sign transfer: %w", err)
	}
	return tx, nil
}

// BuildAndSignLock creates a signed LOCK transaction, locking funds from this
// wallet (the investor) into a contract's escrow, payable to the founder.
func (w *Wallet) BuildAndSignLock(founderAddress, contractID string, amount int64, nonce int64) (*blockchain.Transaction, error) {
	tx := blockchain.NewTransaction(blockchain.TxLock, w.Key.Address, founderAddress, amount, contractID, "", nonce)
	if err := tx.Sign(w.Key.PrivateKey, w.Key.PublicKey); err != nil {
		return nil, fmt.Errorf("failed to sign lock: %w", err)
	}
	return tx, nil
}

// VerifyAddress confirms this wallet's stored address actually matches its
// stored public key - a sanity check after loading a key file from disk.
func (w *Wallet) VerifyAddress() bool {
	expected := crypto.AddressFromPubKeyHex(w.Key.PublicKey)
	return expected == w.Key.Address
}
'@
[System.IO.File]::WriteAllText("$PWD\internal\wallet\wallet.go", $content)

# ── cmd/walletcli/main.go ──
New-Item -ItemType Directory -Path cmd\walletcli -Force | Out-Null
$content = @'
package main

import (
	"fmt"
	"os"

	"github.com/MuaazTasawar/venturechain/internal/wallet"
	"github.com/spf13/cobra"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "walletcli",
		Short: "VentureChain wallet management CLI",
	}

	var label, outPath string
	genCmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate a new wallet key pair and save it to a file",
		Run: func(cmd *cobra.Command, args []string) {
			if outPath == "" {
				outPath = fmt.Sprintf("%s.wallet.json", label)
			}
			key, err := wallet.GenerateAndSave(label, outPath)
			if err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				os.Exit(1)
			}
			fmt.Println("Wallet generated and saved to:", outPath)
			fmt.Println("Label:      ", key.Label)
			fmt.Println("Address:    ", key.Address)
			fmt.Println("Public Key: ", key.PublicKey)
			fmt.Println()
			fmt.Println("Paste the Address and Public Key into config/genesis.json where needed.")
			fmt.Println("Keep the .wallet.json file private — it contains the private key.")
		},
	}
	genCmd.Flags().StringVar(&label, "label", "wallet", "Human-readable label for this wallet")
	genCmd.Flags().StringVar(&outPath, "out", "", "Output file path (default: <label>.wallet.json)")

	var showPath string
	showCmd := &cobra.Command{
		Use:   "show",
		Short: "Show the address and public key stored in a wallet file",
		Run: func(cmd *cobra.Command, args []string) {
			key, err := wallet.Load(showPath)
			if err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				os.Exit(1)
			}
			ok := wallet.New(key).VerifyAddress()
			fmt.Println("Label:      ", key.Label)
			fmt.Println("Address:    ", key.Address)
			fmt.Println("Public Key: ", key.PublicKey)
			fmt.Println("Valid:      ", ok)
		},
	}
	showCmd.Flags().StringVar(&showPath, "file", "", "Path to a .wallet.json file")
	showCmd.MarkFlagRequired("file")

	rootCmd.AddCommand(genCmd, showCmd)
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
'@
[System.IO.File]::WriteAllText("$PWD\cmd\walletcli\main.go", $content)

go build ./...