package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/foxzi/sendry/internal/config"
	"github.com/foxzi/sendry/internal/tls"
)

var tlsCmd = &cobra.Command{
	Use:   "tls",
	Short: "TLS certificate management",
}

var tlsRenewCmd = &cobra.Command{
	Use:   "renew",
	Short: "Renew TLS certificates via ACME",
	Long: `Start temporary HTTP server on port 80 to handle ACME HTTP-01 challenge
and obtain/renew TLS certificates from Let's Encrypt.

The server will automatically stop after certificates are obtained.`,
	RunE: runTLSRenew,
}

var tlsStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show TLS certificate status",
	RunE:  runTLSStatus,
}

var (
	tlsRenewTimeout time.Duration
	tlsForceRenew   bool
)

func init() {
	tlsRenewCmd.Flags().DurationVar(&tlsRenewTimeout, "timeout", 2*time.Minute, "timeout for certificate renewal")
	tlsRenewCmd.Flags().BoolVar(&tlsForceRenew, "force", false, "force renewal even if certificate is valid")

	tlsCmd.AddCommand(tlsRenewCmd, tlsStatusCmd)
	rootCmd.AddCommand(tlsCmd)
}

func runTLSRenew(cmd *cobra.Command, args []string) error {
	if cfgFile == "" {
		return fmt.Errorf("config file is required (use -c flag)")
	}

	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if !cfg.SMTP.TLS.ACME.Enabled {
		return fmt.Errorf("ACME is not enabled in configuration")
	}

	// Create ACME manager
	acmeManager := tls.NewACMEManager(
		cfg.SMTP.TLS.ACME.Email,
		cfg.SMTP.TLS.ACME.Domains,
		cfg.SMTP.TLS.ACME.CacheDir,
	)

	// Check current certificate status first
	if !tlsForceRenew {
		fmt.Println("Checking current certificate status...")
		certs, err := acmeManager.GetCachedCertificates()
		if err == nil && len(certs) > 0 {
			allValid := true
			for _, cert := range certs {
				if cert.DaysLeft < 30 {
					allValid = false
					fmt.Printf("  %s: expires in %d days - renewal needed\n", cert.Domain, cert.DaysLeft)
				} else {
					fmt.Printf("  %s: expires in %d days - OK\n", cert.Domain, cert.DaysLeft)
				}
			}
			if allValid {
				fmt.Println("\nAll certificates are valid. Use --force to renew anyway.")
				return nil
			}
		}
	}

	// Setup context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nReceived interrupt, shutting down...")
		cancel()
	}()

	// Start HTTP server for ACME challenge
	httpServer := &http.Server{
		Addr: ":80",
		Handler: acmeManager.HTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "ACME challenge server", http.StatusNotFound)
		})),
	}

	// Start server in goroutine
	serverErr := make(chan error, 1)
	go func() {
		fmt.Println("Starting ACME HTTP challenge server on :80...")
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
		close(serverErr)
	}()

	// Wait a bit for server to start
	time.Sleep(100 * time.Millisecond)

	// Check if server started successfully
	select {
	case err := <-serverErr:
		if err != nil {
			return fmt.Errorf("failed to start HTTP server: %w (is port 80 available?)", err)
		}
	default:
	}

	// Obtain/renew certificates
	fmt.Println("Obtaining/renewing certificates...")
	certCtx, certCancel := context.WithTimeout(ctx, tlsRenewTimeout)
	certs, err := acmeManager.EnsureCertificates(certCtx)
	certCancel()

	// Shutdown HTTP server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if shutdownErr := httpServer.Shutdown(shutdownCtx); shutdownErr != nil {
		fmt.Printf("Warning: HTTP server shutdown error: %v\n", shutdownErr)
	}
	fmt.Println("ACME HTTP server stopped.")

	if err != nil {
		return fmt.Errorf("failed to obtain certificates: %w", err)
	}

	// Print results
	fmt.Println("\nCertificate status:")
	for _, cert := range certs {
		status := "renewed"
		if !cert.IsNew {
			status = "valid"
		}
		fmt.Printf("  %s: %s (expires in %d days)\n", cert.Domain, status, cert.DaysLeft)
	}

	fmt.Println("\nCertificates are ready. You can now start sendry.")
	return nil
}

func runTLSStatus(cmd *cobra.Command, args []string) error {
	if cfgFile == "" {
		return fmt.Errorf("config file is required (use -c flag)")
	}

	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if !cfg.SMTP.TLS.ACME.Enabled {
		// Check manual certificates
		if cfg.SMTP.TLS.CertFile != "" {
			info, err := tls.GetCertificateInfo(cfg.SMTP.TLS.CertFile)
			if err != nil {
				return fmt.Errorf("failed to read certificate: %w", err)
			}
			fmt.Println("TLS Certificate (manual):")
			fmt.Printf("  File: %s\n", cfg.SMTP.TLS.CertFile)
			fmt.Printf("  Subject: %s\n", info.Subject)
			fmt.Printf("  Issuer: %s\n", info.Issuer)
			fmt.Printf("  Valid from: %s\n", info.NotBefore.Format(time.RFC3339))
			fmt.Printf("  Valid until: %s\n", info.NotAfter.Format(time.RFC3339))
			fmt.Printf("  Days left: %d\n", info.DaysLeft)
			return nil
		}
		fmt.Println("TLS is not configured")
		return nil
	}

	// Check ACME certificates
	acmeManager := tls.NewACMEManager(
		cfg.SMTP.TLS.ACME.Email,
		cfg.SMTP.TLS.ACME.Domains,
		cfg.SMTP.TLS.ACME.CacheDir,
	)

	certs, err := acmeManager.GetCachedCertificates()
	if err != nil {
		return fmt.Errorf("failed to read cached certificates: %w", err)
	}

	if len(certs) == 0 {
		fmt.Println("ACME certificates not found in cache.")
		fmt.Println("Run 'sendry tls renew' to obtain certificates.")
		return nil
	}

	fmt.Println("ACME Certificates:")
	for _, cert := range certs {
		status := "OK"
		if cert.DaysLeft < 30 {
			status = "RENEWAL NEEDED"
		} else if cert.DaysLeft < 14 {
			status = "EXPIRING SOON"
		}
		fmt.Printf("  %s:\n", cert.Domain)
		fmt.Printf("    Valid until: %s\n", cert.NotAfter.Format(time.RFC3339))
		fmt.Printf("    Days left: %d\n", cert.DaysLeft)
		fmt.Printf("    Status: %s\n", status)
	}

	return nil
}
