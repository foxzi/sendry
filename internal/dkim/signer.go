package dkim

import (
	"bytes"
	"crypto"
	"crypto/rsa"
	"fmt"

	"github.com/emersion/go-msgauth/dkim"
)

// Signer signs email messages with DKIM
type Signer struct {
	privateKey *rsa.PrivateKey
	domain     string
	selector   string
}

// NewSigner creates a new DKIM signer
func NewSigner(privateKey *rsa.PrivateKey, domain, selector string) *Signer {
	return &Signer{
		privateKey: privateKey,
		domain:     domain,
		selector:   selector,
	}
}

// NewSignerFromFile creates a new DKIM signer from a key file
func NewSignerFromFile(keyFile, domain, selector string) (*Signer, error) {
	privateKey, err := LoadPrivateKey(keyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load DKIM key: %w", err)
	}

	return NewSigner(privateKey, domain, selector), nil
}

// Sign signs the message and returns the signed message
func (s *Signer) Sign(message []byte) ([]byte, error) {
	options := &dkim.SignOptions{
		Domain:   s.domain,
		Selector: s.selector,
		Signer:   s.privateKey,
		Hash:     crypto.SHA256,
		HeaderCanonicalization: dkim.CanonicalizationRelaxed,
		BodyCanonicalization:   dkim.CanonicalizationRelaxed,
	}

	var signedMsg bytes.Buffer
	if err := dkim.Sign(&signedMsg, bytes.NewReader(message), options); err != nil {
		return nil, fmt.Errorf("failed to sign message: %w", err)
	}

	return signedMsg.Bytes(), nil
}

// Domain returns the DKIM domain
func (s *Signer) Domain() string {
	return s.domain
}

// Selector returns the DKIM selector
func (s *Signer) Selector() string {
	return s.selector
}
