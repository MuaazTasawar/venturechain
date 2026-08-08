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
			fmt.Println("Keep the .wallet.json file private â€” it contains the private key.")
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