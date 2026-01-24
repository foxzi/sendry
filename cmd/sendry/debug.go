package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/foxzi/sendry/internal/config"
)

var (
	debugDomain    string
	debugCheckOnly string
)

var debugCmd = &cobra.Command{
	Use:   "debug",
	Short: "System diagnostics and debugging",
	RunE:  runDebug,
}

func init() {
	debugCmd.Flags().StringVar(&debugDomain, "domain", "", "Check specific domain")
	debugCmd.Flags().StringVar(&debugCheckOnly, "check", "", "Run specific check only (ports, dns, tls, dkim, connectivity)")

	rootCmd.AddCommand(debugCmd)
}

func runDebug(cmd *cobra.Command, args []string) error {
	fmt.Println("Sendry System Diagnostics")
	fmt.Println("=========================")
	fmt.Println()

	// System info
	fmt.Println("System Information:")
	fmt.Printf("  Go Version: %s\n", runtime.Version())
	fmt.Printf("  OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Printf("  CPUs: %d\n", runtime.NumCPU())
	fmt.Println()

	if cfgFile == "" {
		fmt.Println("Note: No config file specified. Some checks will be skipped.")
		fmt.Println("Use -c flag to specify config file for full diagnostics.")
		fmt.Println()

		// Run basic checks without config
		if debugCheckOnly == "" || debugCheckOnly == "ports" {
			checkBasicPorts()
		}
		return nil
	}

	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	fmt.Printf("Configuration: %s\n", cfgFile)
	fmt.Printf("Hostname: %s\n", cfg.Server.Hostname)
	fmt.Println()

	runAll := debugCheckOnly == ""

	// Port checks
	if runAll || debugCheckOnly == "ports" {
		checkPorts(cfg)
	}

	// DNS checks
	if runAll || debugCheckOnly == "dns" {
		domain := debugDomain
		if domain == "" {
			domain = cfg.SMTP.Domain
		}
		checkDNS(domain)
	}

	// TLS checks
	if runAll || debugCheckOnly == "tls" {
		checkTLS(cfg)
	}

	// DKIM checks
	if runAll || debugCheckOnly == "dkim" {
		checkDKIM(cfg, debugDomain)
	}

	// Connectivity checks
	if runAll || debugCheckOnly == "connectivity" {
		checkConnectivity()
	}

	// Storage checks
	if runAll {
		checkStorage(cfg)
	}

	return nil
}

func checkBasicPorts() {
	fmt.Println("Port Checks:")

	ports := []struct {
		port int
		name string
	}{
		{25, "SMTP"},
		{465, "SMTPS"},
		{587, "Submission"},
		{8080, "HTTP API"},
	}

	for _, p := range ports {
		addr := fmt.Sprintf("localhost:%d", p.port)
		conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err != nil {
			fmt.Printf("  [--] Port %d (%s): not listening\n", p.port, p.name)
		} else {
			conn.Close()
			fmt.Printf("  [OK] Port %d (%s): listening\n", p.port, p.name)
		}
	}
	fmt.Println()
}

func checkPorts(cfg *config.Config) {
	fmt.Println("Port Checks:")

	ports := []struct {
		addr string
		name string
	}{
		{cfg.SMTP.ListenAddr, "SMTP"},
		{cfg.SMTP.SMTPSAddr, "SMTPS"},
		{cfg.SMTP.SubmissionAddr, "Submission"},
		{cfg.API.ListenAddr, "HTTP API"},
	}

	for _, p := range ports {
		addr := p.addr
		if strings.HasPrefix(addr, ":") {
			addr = "localhost" + addr
		}

		conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err != nil {
			fmt.Printf("  [--] %s (%s): not listening\n", p.name, p.addr)
		} else {
			conn.Close()
			fmt.Printf("  [OK] %s (%s): listening\n", p.name, p.addr)
		}
	}
	fmt.Println()
}

func checkDNS(domain string) {
	fmt.Printf("DNS Checks for %s:\n", domain)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// MX records
	mxRecords, err := net.DefaultResolver.LookupMX(ctx, domain)
	if err != nil {
		fmt.Printf("  [WARN] MX: lookup failed: %v\n", err)
	} else if len(mxRecords) == 0 {
		fmt.Printf("  [WARN] MX: no records found\n")
	} else {
		fmt.Printf("  [OK] MX: %d record(s) found\n", len(mxRecords))
	}

	// SPF
	txtRecords, err := net.DefaultResolver.LookupTXT(ctx, domain)
	if err != nil {
		fmt.Printf("  [WARN] SPF: lookup failed: %v\n", err)
	} else {
		hasSPF := false
		for _, txt := range txtRecords {
			if strings.HasPrefix(txt, "v=spf1") {
				hasSPF = true
				break
			}
		}
		if hasSPF {
			fmt.Printf("  [OK] SPF: record found\n")
		} else {
			fmt.Printf("  [WARN] SPF: no record found\n")
		}
	}

	// DMARC
	dmarcRecords, err := net.DefaultResolver.LookupTXT(ctx, "_dmarc."+domain)
	if err != nil {
		fmt.Printf("  [WARN] DMARC: lookup failed\n")
	} else {
		hasDMARC := false
		for _, txt := range dmarcRecords {
			if strings.HasPrefix(txt, "v=DMARC1") {
				hasDMARC = true
				break
			}
		}
		if hasDMARC {
			fmt.Printf("  [OK] DMARC: record found\n")
		} else {
			fmt.Printf("  [WARN] DMARC: no record found\n")
		}
	}

	fmt.Println()
}

func checkTLS(cfg *config.Config) {
	fmt.Println("TLS Checks:")

	hasTLS := cfg.HasTLS()
	if !hasTLS {
		fmt.Println("  [WARN] TLS is not configured")
		fmt.Println()
		return
	}

	if cfg.SMTP.TLS.ACME.Enabled {
		fmt.Println("  [OK] ACME (Let's Encrypt) is enabled")
		fmt.Printf("  ACME domains: %v\n", cfg.SMTP.TLS.ACME.Domains)
		fmt.Printf("  ACME cache: %s\n", cfg.SMTP.TLS.ACME.CacheDir)

		// Check if cache dir exists
		if _, err := os.Stat(cfg.SMTP.TLS.ACME.CacheDir); os.IsNotExist(err) {
			fmt.Printf("  [WARN] ACME cache directory does not exist\n")
		}
	} else {
		fmt.Println("  [OK] Manual certificates configured")
		fmt.Printf("  Cert file: %s\n", cfg.SMTP.TLS.CertFile)
		fmt.Printf("  Key file: %s\n", cfg.SMTP.TLS.KeyFile)

		// Check if files exist
		if _, err := os.Stat(cfg.SMTP.TLS.CertFile); os.IsNotExist(err) {
			fmt.Printf("  [ERR] Certificate file not found\n")
		}
		if _, err := os.Stat(cfg.SMTP.TLS.KeyFile); os.IsNotExist(err) {
			fmt.Printf("  [ERR] Key file not found\n")
		}
	}

	// Try to connect with TLS to submission port
	submissionAddr := cfg.SMTP.SubmissionAddr
	if strings.HasPrefix(submissionAddr, ":") {
		submissionAddr = "localhost" + submissionAddr
	}

	conn, err := net.DialTimeout("tcp", submissionAddr, 5*time.Second)
	if err == nil {
		tlsConn := tls.Client(conn, &tls.Config{
			InsecureSkipVerify: true,
			ServerName:         cfg.Server.Hostname,
		})
		defer tlsConn.Close()

		if err := tlsConn.Handshake(); err != nil {
			fmt.Printf("  [WARN] TLS handshake failed on submission port: %v\n", err)
		} else {
			state := tlsConn.ConnectionState()
			fmt.Printf("  [OK] TLS working on submission port (TLS %s)\n", tlsVersionString(state.Version))
		}
	}

	fmt.Println()
}

func checkDKIM(cfg *config.Config, specificDomain string) {
	fmt.Println("DKIM Checks:")

	domains := cfg.GetAllDomains()
	if specificDomain != "" {
		domains = []string{specificDomain}
	}

	for _, domain := range domains {
		enabled, selector, keyFile := cfg.GetDKIMConfig(domain)

		if !enabled {
			fmt.Printf("  [--] %s: DKIM not enabled\n", domain)
			continue
		}

		// Check key file exists
		if _, err := os.Stat(keyFile); os.IsNotExist(err) {
			fmt.Printf("  [ERR] %s: key file not found: %s\n", domain, keyFile)
			continue
		}

		// Check DNS record
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		dkimDomain := fmt.Sprintf("%s._domainkey.%s", selector, domain)
		txtRecords, err := net.DefaultResolver.LookupTXT(ctx, dkimDomain)
		cancel()

		if err != nil {
			fmt.Printf("  [WARN] %s: DNS lookup failed for %s\n", domain, dkimDomain)
			continue
		}

		hasDKIM := false
		for _, txt := range txtRecords {
			if strings.Contains(txt, "v=DKIM1") {
				hasDKIM = true
				break
			}
		}

		if hasDKIM {
			fmt.Printf("  [OK] %s: configured (selector: %s)\n", domain, selector)
		} else {
			fmt.Printf("  [WARN] %s: key file exists but no DNS record found\n", domain)
		}
	}

	fmt.Println()
}

func checkConnectivity() {
	fmt.Println("Connectivity Checks:")

	// Test common mail servers
	servers := []string{
		"gmail-smtp-in.l.google.com:25",
		"mx.yandex.ru:25",
	}

	for _, server := range servers {
		conn, err := net.DialTimeout("tcp", server, 5*time.Second)
		if err != nil {
			fmt.Printf("  [WARN] %s: connection failed\n", server)
		} else {
			conn.Close()
			fmt.Printf("  [OK] %s: reachable\n", server)
		}
	}

	fmt.Println()
}

func checkStorage(cfg *config.Config) {
	fmt.Println("Storage Checks:")

	// Check storage path
	storageDir := cfg.Storage.Path
	if storageDir == "" {
		fmt.Println("  [WARN] Storage path not configured")
		fmt.Println()
		return
	}

	// Check if storage file exists
	if info, err := os.Stat(storageDir); err == nil {
		fmt.Printf("  [OK] Storage file exists: %s\n", storageDir)
		fmt.Printf("  Size: %d bytes\n", info.Size())
	} else if os.IsNotExist(err) {
		fmt.Printf("  [--] Storage file does not exist yet: %s\n", storageDir)
	} else {
		fmt.Printf("  [ERR] Cannot access storage: %v\n", err)
	}

	fmt.Println()
}
