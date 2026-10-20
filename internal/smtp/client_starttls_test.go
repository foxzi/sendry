package smtp

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"log/slog"
	"math/big"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	gosmtp "github.com/emersion/go-smtp"
	"github.com/foxzi/sendry/internal/dns"
)

// fakeMX records connections and delivered messages
type fakeMX struct {
	mu          sync.Mutex
	connections int
	delivered   []string
}

func (b *fakeMX) NewSession(c *gosmtp.Conn) (gosmtp.Session, error) {
	b.mu.Lock()
	b.connections++
	b.mu.Unlock()
	return &fakeMXSession{backend: b}, nil
}

type fakeMXSession struct {
	backend *fakeMX
	rcpts   []string
}

func (s *fakeMXSession) Mail(from string, opts *gosmtp.MailOptions) error { return nil }

func (s *fakeMXSession) Rcpt(to string, opts *gosmtp.RcptOptions) error {
	s.rcpts = append(s.rcpts, to)
	return nil
}

func (s *fakeMXSession) Data(r io.Reader) error {
	if _, err := io.ReadAll(r); err != nil {
		return err
	}
	s.backend.mu.Lock()
	s.backend.delivered = append(s.backend.delivered, s.rcpts...)
	s.backend.mu.Unlock()
	return nil
}

func (s *fakeMXSession) Reset()        { s.rcpts = nil }
func (s *fakeMXSession) Logout() error { return nil }

// selfSignedCert returns a certificate the client will not trust
func selfSignedCert(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "mx.test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{"mx.test"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

func TestSendToMXFallsBackWhenSTARTTLSFails(t *testing.T) {
	backend := &fakeMX{}
	server := gosmtp.NewServer(backend)
	server.Domain = "mx.test"
	server.TLSConfig = &tls.Config{Certificates: []tls.Certificate{selfSignedCert(t)}}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go server.Serve(ln)
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	client := NewClient(dns.NewResolver(0), "mail.example.com", 5*time.Second, logger)
	_, client.port, _ = net.SplitHostPort(ln.Addr().String())

	err = client.sendToMX(context.Background(), "127.0.0.1", "sender@example.com",
		[]string{"rcpt@example.org"}, []byte("Subject: test\r\n\r\nhello\r\n"))
	if err != nil {
		t.Fatalf("sendToMX() error = %v, want delivery over plain connection", err)
	}

	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.connections != 2 {
		t.Errorf("connections = %d, want 2 (failed STARTTLS + plain retry)", backend.connections)
	}
	if len(backend.delivered) != 1 || backend.delivered[0] != "rcpt@example.org" {
		t.Errorf("delivered = %v, want [rcpt@example.org]", backend.delivered)
	}
}
