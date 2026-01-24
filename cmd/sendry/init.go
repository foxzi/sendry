package main

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/foxzi/sendry/internal/dkim"
)

var (
	initDomain    string
	initHostname  string
	initOutput    string
	initDKIM      bool
	initDKIMDir   string
	initAPIKey    string
	initSMTPUser  string
	initSMTPPass  string
	initDataDir   string
	initMode      string
	initACME      bool
	initACMEEmail string
	initForce     bool
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize Sendry configuration",
	Long: `Interactive wizard to create a Sendry configuration file.

This command helps you set up Sendry by:
  1. Creating a configuration file
  2. Optionally generating DKIM keys
  3. Showing DNS records to add (SPF, DKIM, DMARC)

Examples:
  # Interactive mode - prompts for missing values
  sendry init

  # Non-interactive with all flags
  sendry init --domain example.com --hostname mail.example.com --dkim

  # Quick setup for testing
  sendry init --domain test.local --mode sandbox -o test.yaml`,
	RunE: runInit,
}

func init() {
	initCmd.Flags().StringVar(&initDomain, "domain", "", "Mail domain (e.g., example.com)")
	initCmd.Flags().StringVar(&initHostname, "hostname", "", "Server hostname FQDN (default: mail.<domain>)")
	initCmd.Flags().StringVarP(&initOutput, "output", "o", "config.yaml", "Output configuration file path")
	initCmd.Flags().BoolVar(&initDKIM, "dkim", false, "Generate DKIM keys")
	initCmd.Flags().StringVar(&initDKIMDir, "dkim-dir", "", "DKIM keys directory (default: <data-dir>/dkim)")
	initCmd.Flags().StringVar(&initAPIKey, "api-key", "", "API key (auto-generated if not provided)")
	initCmd.Flags().StringVar(&initSMTPUser, "smtp-user", "", "SMTP auth username (default: admin)")
	initCmd.Flags().StringVar(&initSMTPPass, "smtp-pass", "", "SMTP auth password (auto-generated if not provided)")
	initCmd.Flags().StringVar(&initDataDir, "data-dir", "/var/lib/sendry", "Data directory for queue and keys")
	initCmd.Flags().StringVar(&initMode, "mode", "production", "Domain mode: production, sandbox, redirect")
	initCmd.Flags().BoolVar(&initACME, "acme", false, "Enable Let's Encrypt TLS")
	initCmd.Flags().StringVar(&initACMEEmail, "acme-email", "", "Email for Let's Encrypt account")
	initCmd.Flags().BoolVar(&initForce, "force", false, "Overwrite existing config file")

	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("Sendry Configuration Wizard")
	fmt.Println("===========================")
	fmt.Println()

	// Get domain (required)
	if initDomain == "" {
		initDomain = prompt(reader, "Mail domain (e.g., example.com)", "")
		if initDomain == "" {
			return fmt.Errorf("domain is required")
		}
	}

	// Get hostname (default to mail.<domain>)
	if initHostname == "" {
		defaultHostname := "mail." + initDomain
		initHostname = prompt(reader, "Server hostname", defaultHostname)
	}

	// Get data directory
	initDataDir = prompt(reader, "Data directory", initDataDir)

	// DKIM setup
	if !initDKIM {
		answer := prompt(reader, "Generate DKIM keys? [y/N]", "n")
		initDKIM = strings.ToLower(answer) == "y" || strings.ToLower(answer) == "yes"
	}

	if initDKIMDir == "" {
		initDKIMDir = filepath.Join(initDataDir, "dkim")
	}

	// ACME setup
	if !initACME {
		answer := prompt(reader, "Enable Let's Encrypt TLS? [y/N]", "n")
		initACME = strings.ToLower(answer) == "y" || strings.ToLower(answer) == "yes"
	}

	if initACME && initACMEEmail == "" {
		initACMEEmail = prompt(reader, "Email for Let's Encrypt", "admin@"+initDomain)
	}

	// SMTP auth
	if initSMTPUser == "" {
		initSMTPUser = prompt(reader, "SMTP username", "admin")
	}

	if initSMTPPass == "" {
		initSMTPPass = generateRandomString(16)
		fmt.Printf("  Generated SMTP password: %s\n", initSMTPPass)
	}

	// API key
	if initAPIKey == "" {
		initAPIKey = generateRandomString(32)
		fmt.Printf("  Generated API key: %s\n", initAPIKey)
	}

	// Check if output file exists
	if !initForce {
		if _, err := os.Stat(initOutput); err == nil {
			return fmt.Errorf("config file %s already exists (use --force to overwrite)", initOutput)
		}
	}

	fmt.Println()
	fmt.Println("Creating configuration...")

	// Create directories if needed
	if err := os.MkdirAll(initDataDir, 0755); err != nil {
		fmt.Printf("  Warning: Could not create data directory: %v\n", err)
	}

	// Generate DKIM keys if requested
	var dkimKeyPath string
	var dkimDNSRecord string
	var dkimDNSName string

	if initDKIM {
		if err := os.MkdirAll(initDKIMDir, 0700); err != nil {
			return fmt.Errorf("failed to create DKIM directory: %w", err)
		}

		kp, err := dkim.GenerateKey(initDomain, "sendry")
		if err != nil {
			return fmt.Errorf("failed to generate DKIM key: %w", err)
		}

		dkimKeyPath = filepath.Join(initDKIMDir, initDomain+".key")
		if err := kp.SavePrivateKey(dkimKeyPath); err != nil {
			return fmt.Errorf("failed to save DKIM key: %w", err)
		}

		dkimDNSRecord = kp.DNSRecord()
		dkimDNSName = kp.DNSName()
		fmt.Printf("  DKIM key saved to: %s\n", dkimKeyPath)
	}

	// Generate config
	config := generateConfig(dkimKeyPath)

	// Write config file
	if err := os.WriteFile(initOutput, []byte(config), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	fmt.Printf("  Configuration saved to: %s\n", initOutput)
	fmt.Println()

	// Print DNS records
	printDNSRecords(dkimDNSName, dkimDNSRecord)

	// Print next steps
	printNextSteps()

	return nil
}

func prompt(reader *bufio.Reader, question, defaultValue string) string {
	if defaultValue != "" {
		fmt.Printf("%s [%s]: ", question, defaultValue)
	} else {
		fmt.Printf("%s: ", question)
	}

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "" {
		return defaultValue
	}
	return input
}

func generateRandomString(length int) string {
	bytes := make([]byte, length/2)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

func generateConfig(dkimKeyPath string) string {
	acmeSection := ""
	if initACME {
		acmeSection = fmt.Sprintf(`    acme:
      enabled: true
      email: "%s"
      domains:
        - "%s"
      cache_dir: "%s/certs"`, initACMEEmail, initHostname, initDataDir)
	} else {
		acmeSection = `    # Uncomment to enable Let's Encrypt
    # acme:
    #   enabled: true
    #   email: "admin@` + initDomain + `"
    #   domains:
    #     - "` + initHostname + `"
    #   cache_dir: "` + initDataDir + `/certs"`
	}

	dkimSection := ""
	if initDKIM && dkimKeyPath != "" {
		dkimSection = fmt.Sprintf(`dkim:
  enabled: true
  selector: "sendry"
  domain: "%s"
  key_file: "%s"`, initDomain, dkimKeyPath)
	} else {
		dkimSection = fmt.Sprintf(`dkim:
  enabled: false
  selector: "sendry"
  domain: "%s"
  key_file: "%s/dkim/%s.key"`, initDomain, initDataDir, initDomain)
	}

	domainDKIM := ""
	if initDKIM && dkimKeyPath != "" {
		domainDKIM = fmt.Sprintf(`    dkim:
      enabled: true
      selector: "sendry"
      key_file: "%s"`, dkimKeyPath)
	} else {
		domainDKIM = fmt.Sprintf(`    dkim:
      enabled: false
      selector: "sendry"
      key_file: "%s/dkim/%s.key"`, initDataDir, initDomain)
	}

	return fmt.Sprintf(`# Sendry configuration
# Generated by: sendry init

server:
  hostname: "%s"

smtp:
  listen_addr: ":25"
  submission_addr: ":587"
  smtps_addr: ":465"
  domain: "%s"
  max_message_bytes: 10485760  # 10MB
  max_recipients: 100
  auth:
    required: true
    users:
      %s: "%s"
    # Brute force protection
    max_failures: 5        # Max auth failures before blocking
    block_duration: 15m    # How long to block after max failures
    failure_window: 5m     # Window for counting failures
  tls:
%s

%s

domains:
  %s:
%s
    mode: %s
    rate_limit:
      messages_per_hour: 1000
      messages_per_day: 10000

rate_limit:
  enabled: true
  global:
    messages_per_hour: 50000
    messages_per_day: 500000

api:
  listen_addr: ":8080"
  api_key: "%s"
  max_header_bytes: 1048576  # 1 MB
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 60s

queue:
  workers: 4
  retry_interval: 5m
  max_retries: 5
  process_interval: 10s

storage:
  path: "%s/queue.db"

logging:
  level: "info"
  format: "json"
`,
		initHostname,
		initDomain,
		initSMTPUser, initSMTPPass,
		acmeSection,
		dkimSection,
		initDomain,
		domainDKIM,
		initMode,
		initAPIKey,
		initDataDir,
	)
}

func printDNSRecords(dkimDNSName, dkimDNSRecord string) {
	fmt.Println("DNS Records to Add")
	fmt.Println("==================")
	fmt.Println()

	// A record
	fmt.Println("1. A Record (point your hostname to server IP):")
	fmt.Printf("   Name:  %s\n", initHostname)
	fmt.Printf("   Type:  A\n")
	fmt.Printf("   Value: <your-server-ip>\n")
	fmt.Println()

	// MX record
	fmt.Println("2. MX Record (for receiving mail / bounce handling):")
	fmt.Printf("   Name:  %s\n", initDomain)
	fmt.Printf("   Type:  MX\n")
	fmt.Printf("   Value: 10 %s\n", initHostname)
	fmt.Println()

	// SPF record
	fmt.Println("3. SPF Record (authorize your server to send mail):")
	fmt.Printf("   Name:  %s\n", initDomain)
	fmt.Printf("   Type:  TXT\n")
	fmt.Printf("   Value: v=spf1 mx a:%s ~all\n", initHostname)
	fmt.Println()

	// DKIM record
	if dkimDNSName != "" && dkimDNSRecord != "" {
		fmt.Println("4. DKIM Record (email signing):")
		fmt.Printf("   Name:  %s\n", dkimDNSName)
		fmt.Printf("   Type:  TXT\n")
		fmt.Printf("   Value: %s\n", dkimDNSRecord)
		fmt.Println()
	}

	// DMARC record
	fmt.Println("5. DMARC Record (email policy):")
	fmt.Printf("   Name:  _dmarc.%s\n", initDomain)
	fmt.Printf("   Type:  TXT\n")
	fmt.Printf("   Value: v=DMARC1; p=quarantine; rua=mailto:dmarc@%s\n", initDomain)
	fmt.Println()

	// PTR record
	fmt.Println("6. PTR Record (reverse DNS - configure at your hosting provider):")
	fmt.Printf("   IP:    <your-server-ip>\n")
	fmt.Printf("   Value: %s\n", initHostname)
	fmt.Println()
}

func printNextSteps() {
	fmt.Println("Next Steps")
	fmt.Println("==========")
	fmt.Println()
	fmt.Println("1. Add the DNS records shown above")
	fmt.Println()
	fmt.Println("2. Verify DNS propagation:")
	fmt.Printf("   sendry dns check %s --all\n", initDomain)
	fmt.Println()
	fmt.Println("3. Start the server:")
	fmt.Printf("   sendry serve -c %s\n", initOutput)
	fmt.Println()
	fmt.Println("4. Test sending:")
	fmt.Println("   curl -X POST http://localhost:8080/api/v1/send \\")
	fmt.Printf("     -H \"Authorization: Bearer %s\" \\\n", initAPIKey)
	fmt.Println("     -H \"Content-Type: application/json\" \\")
	fmt.Printf("     -d '{\"from\": \"test@%s\", \"to\": [\"your@email.com\"], \"subject\": \"Test\", \"body\": \"Hello!\"}'\n", initDomain)
	fmt.Println()
	fmt.Println("Credentials")
	fmt.Println("-----------")
	fmt.Printf("SMTP User:     %s\n", initSMTPUser)
	fmt.Printf("SMTP Password: %s\n", initSMTPPass)
	fmt.Printf("API Key:       %s\n", initAPIKey)
	fmt.Println()
}
