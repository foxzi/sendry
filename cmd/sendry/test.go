package main

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/foxzi/sendry/internal/config"
)

var (
	testSendTo      string
	testSendFrom    string
	testSendSubject string
	testSendBody    string
	testSMTPHost    string
	testSMTPPort    int
	testSMTPTimeout int
)

var testCmd = &cobra.Command{
	Use:   "test",
	Short: "Testing and debugging commands",
}

var testSendCmd = &cobra.Command{
	Use:   "send",
	Short: "Send a test email",
	RunE:  runTestSend,
}

var testSMTPCmd = &cobra.Command{
	Use:   "smtp",
	Short: "Test SMTP connection to a server",
	RunE:  runTestSMTP,
}

func init() {
	testSendCmd.Flags().StringVar(&testSendTo, "to", "", "Recipient email address (required)")
	testSendCmd.Flags().StringVar(&testSendFrom, "from", "", "Sender email address")
	testSendCmd.Flags().StringVar(&testSendSubject, "subject", "Test message from Sendry", "Email subject")
	testSendCmd.Flags().StringVar(&testSendBody, "body", "This is a test message sent from Sendry MTA.", "Email body")
	testSendCmd.MarkFlagRequired("to")

	testSMTPCmd.Flags().StringVar(&testSMTPHost, "host", "", "SMTP server hostname (required)")
	testSMTPCmd.Flags().IntVar(&testSMTPPort, "port", 25, "SMTP server port")
	testSMTPCmd.Flags().IntVar(&testSMTPTimeout, "timeout", 10, "Connection timeout in seconds")
	testSMTPCmd.MarkFlagRequired("host")

	testCmd.AddCommand(testSendCmd, testSMTPCmd)
	rootCmd.AddCommand(testCmd)
}

func runTestSend(cmd *cobra.Command, args []string) error {
	if cfgFile == "" {
		return fmt.Errorf("config file is required (use -c flag)")
	}

	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Determine from address
	from := testSendFrom
	if from == "" {
		from = fmt.Sprintf("test@%s", cfg.SMTP.Domain)
	}

	// Build email message
	msg := buildTestMessage(from, testSendTo, testSendSubject, testSendBody)

	fmt.Printf("Sending test email...\n")
	fmt.Printf("  From: %s\n", from)
	fmt.Printf("  To: %s\n", testSendTo)
	fmt.Printf("  Subject: %s\n", testSendSubject)
	fmt.Println()

	// Connect to local SMTP server
	addr := cfg.SMTP.SubmissionAddr
	if !strings.HasPrefix(addr, ":") {
		addr = "localhost" + addr
	} else {
		addr = "localhost" + addr
	}

	fmt.Printf("Connecting to %s...\n", addr)

	// Dial with timeout
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	client, err := smtp.NewClient(conn, "localhost")
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to create SMTP client: %w", err)
	}
	defer client.Close()

	// Try STARTTLS if available
	if ok, _ := client.Extension("STARTTLS"); ok {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: true,
			ServerName:         cfg.Server.Hostname,
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			fmt.Printf("Warning: STARTTLS failed: %v\n", err)
		} else {
			fmt.Println("STARTTLS: OK")
		}
	}

	// Auth if required
	if cfg.SMTP.Auth.Required {
		// Get first user from config
		var username, password string
		for u, p := range cfg.SMTP.Auth.Users {
			username = u
			password = p
			break
		}

		if username != "" {
			auth := smtp.PlainAuth("", username, password, "localhost")
			if err := client.Auth(auth); err != nil {
				return fmt.Errorf("authentication failed: %w", err)
			}
			fmt.Printf("Auth: OK (user: %s)\n", username)
		}
	}

	// Send email
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("MAIL FROM failed: %w", err)
	}

	if err := client.Rcpt(testSendTo); err != nil {
		return fmt.Errorf("RCPT TO failed: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("DATA failed: %w", err)
	}

	_, err = w.Write([]byte(msg))
	if err != nil {
		return fmt.Errorf("write failed: %w", err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("close failed: %w", err)
	}

	if err := client.Quit(); err != nil {
		fmt.Printf("Warning: QUIT failed: %v\n", err)
	}

	fmt.Println()
	fmt.Println("Test email sent successfully!")
	fmt.Println("Check queue status with: sendry queue list")

	return nil
}

func runTestSMTP(cmd *cobra.Command, args []string) error {
	addr := fmt.Sprintf("%s:%d", testSMTPHost, testSMTPPort)
	timeout := time.Duration(testSMTPTimeout) * time.Second

	fmt.Printf("Testing SMTP connection to %s\n\n", addr)

	// DNS lookup
	fmt.Print("DNS lookup... ")
	ips, err := net.LookupIP(testSMTPHost)
	if err != nil {
		fmt.Printf("FAILED: %v\n", err)
	} else {
		var ipStrs []string
		for _, ip := range ips {
			if ip.To4() != nil {
				ipStrs = append(ipStrs, ip.String())
			}
		}
		fmt.Printf("OK (%s)\n", strings.Join(ipStrs, ", "))
	}

	// TCP connection
	fmt.Print("TCP connection... ")
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		fmt.Printf("FAILED: %v\n", err)
		return nil
	}
	latency := time.Since(start)
	fmt.Printf("OK (%v)\n", latency.Round(time.Millisecond))

	// SMTP handshake
	fmt.Print("SMTP banner... ")
	conn.SetDeadline(time.Now().Add(timeout))
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		fmt.Printf("FAILED: %v\n", err)
		conn.Close()
		return nil
	}
	banner := strings.TrimSpace(string(buf[:n]))
	fmt.Printf("OK\n  %s\n", banner)

	// Create SMTP client
	client, err := smtp.NewClient(conn, testSMTPHost)
	if err != nil {
		fmt.Printf("SMTP client error: %v\n", err)
		conn.Close()
		return nil
	}
	defer client.Close()

	// EHLO
	fmt.Print("EHLO... ")
	hostname, _ := getLocalHostname()
	if err := client.Hello(hostname); err != nil {
		fmt.Printf("FAILED: %v\n", err)
	} else {
		fmt.Println("OK")
	}

	// Check extensions
	fmt.Println("\nSupported extensions:")

	extensions := []string{"STARTTLS", "AUTH", "8BITMIME", "SIZE", "PIPELINING", "ENHANCEDSTATUSCODES"}
	for _, ext := range extensions {
		ok, params := client.Extension(ext)
		if ok {
			if params != "" {
				fmt.Printf("  [OK] %s: %s\n", ext, params)
			} else {
				fmt.Printf("  [OK] %s\n", ext)
			}
		} else {
			fmt.Printf("  [--] %s\n", ext)
		}
	}

	// Test STARTTLS
	if ok, _ := client.Extension("STARTTLS"); ok {
		fmt.Print("\nTesting STARTTLS... ")
		tlsConfig := &tls.Config{
			ServerName:         testSMTPHost,
			InsecureSkipVerify: true,
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			fmt.Printf("FAILED: %v\n", err)
		} else {
			state, ok := client.TLSConnectionState()
			if ok {
				fmt.Printf("OK (TLS %s)\n", tlsVersionString(state.Version))
			} else {
				fmt.Println("OK")
			}
		}
	}

	fmt.Println("\nSMTP test completed")
	return nil
}

func buildTestMessage(from, to, subject, body string) string {
	now := time.Now().Format(time.RFC1123Z)

	return fmt.Sprintf(`From: %s
To: %s
Subject: %s
Date: %s
MIME-Version: 1.0
Content-Type: text/plain; charset=utf-8
X-Mailer: Sendry Test

%s
`, from, to, subject, now, body)
}

func getLocalHostname() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "localhost", err
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				names, err := net.LookupAddr(ipnet.IP.String())
				if err == nil && len(names) > 0 {
					return strings.TrimSuffix(names[0], "."), nil
				}
			}
		}
	}

	return "localhost", nil
}

func tlsVersionString(version uint16) string {
	switch version {
	case tls.VersionTLS10:
		return "1.0"
	case tls.VersionTLS11:
		return "1.1"
	case tls.VersionTLS12:
		return "1.2"
	case tls.VersionTLS13:
		return "1.3"
	default:
		return fmt.Sprintf("0x%04x", version)
	}
}
