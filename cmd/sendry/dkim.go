package main

import (
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/foxzi/sendry/internal/dkim"
)

var (
	dkimDomain   string
	dkimSelector string
	dkimKeyFile  string
	dkimOutDir   string
)

var dkimCmd = &cobra.Command{
	Use:   "dkim",
	Short: "DKIM key management commands",
}

var dkimGenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate a new DKIM key pair",
	Long:  `Generate a new RSA 2048-bit DKIM key pair and output DNS record.`,
	RunE:  runDKIMGenerate,
}

var dkimShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show DKIM DNS record from existing key",
	Long:  `Show the DNS TXT record for an existing DKIM private key.`,
	RunE:  runDKIMShow,
}

func init() {
	dkimGenerateCmd.Flags().StringVar(&dkimDomain, "domain", "", "Domain name (required)")
	dkimGenerateCmd.Flags().StringVar(&dkimSelector, "selector", "sendry", "DKIM selector")
	dkimGenerateCmd.Flags().StringVar(&dkimOutDir, "out", ".", "Output directory for key file")
	dkimGenerateCmd.MarkFlagRequired("domain")

	dkimShowCmd.Flags().StringVar(&dkimKeyFile, "key", "", "Path to private key file (required)")
	dkimShowCmd.Flags().StringVar(&dkimDomain, "domain", "", "Domain name (required)")
	dkimShowCmd.Flags().StringVar(&dkimSelector, "selector", "sendry", "DKIM selector")
	dkimShowCmd.MarkFlagRequired("key")
	dkimShowCmd.MarkFlagRequired("domain")

	dkimCmd.AddCommand(dkimGenerateCmd, dkimShowCmd)
	rootCmd.AddCommand(dkimCmd)
}

func runDKIMGenerate(cmd *cobra.Command, args []string) error {
	kp, err := dkim.GenerateKey(dkimDomain, dkimSelector)
	if err != nil {
		return fmt.Errorf("failed to generate key: %w", err)
	}

	keyPath := filepath.Join(dkimOutDir, fmt.Sprintf("%s.key", dkimDomain))
	if err := kp.SavePrivateKey(keyPath); err != nil {
		return fmt.Errorf("failed to save private key: %w", err)
	}

	fmt.Printf("DKIM key generated successfully\n\n")
	fmt.Printf("Private key saved to: %s\n\n", keyPath)
	fmt.Printf("DNS Record:\n")
	fmt.Printf("  Name: %s\n", kp.DNSName())
	fmt.Printf("  Type: TXT\n")
	fmt.Printf("  Value: %s\n", kp.DNSRecord())

	return nil
}

func runDKIMShow(cmd *cobra.Command, args []string) error {
	data, err := os.ReadFile(dkimKeyFile)
	if err != nil {
		return fmt.Errorf("failed to read key file: %w", err)
	}

	privateKey, err := dkim.LoadPrivateKey(dkimKeyFile)
	if err != nil {
		return fmt.Errorf("failed to parse private key: %w", err)
	}

	pubKeyBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return fmt.Errorf("failed to marshal public key: %w", err)
	}
	pubKeyBase64 := base64.StdEncoding.EncodeToString(pubKeyBytes)

	fmt.Printf("DKIM DNS Record:\n\n")
	fmt.Printf("  Name: %s._domainkey.%s\n", dkimSelector, dkimDomain)
	fmt.Printf("  Type: TXT\n")
	fmt.Printf("  Value: v=DKIM1; k=rsa; p=%s\n", pubKeyBase64)

	_ = data // silence unused variable warning

	return nil
}
