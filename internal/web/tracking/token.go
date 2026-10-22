// Package tracking builds and verifies open/click/unsubscribe tracking links
// and rewrites outgoing HTML. It has no database access.
package tracking

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/foxzi/sendry/internal/web/models"
)

// Kind is what a token is for
type Kind byte

const (
	KindOpen        Kind = 'o'
	KindClick       Kind = 'c'
	KindUnsubscribe Kind = 'u'
)

// ErrBadToken is returned for malformed tokens or a bad signature
var ErrBadToken = errors.New("tracking: bad token")

// Payload is what a token carries. SourceType is models.SourceJobItem or models.SourceSend.
type Payload struct {
	Kind       Kind
	SourceType string
	SourceID   string
	LinkID     int    // click only: data-sl number
	URL        string // click only: final destination URL
}

// Signer encodes and verifies tokens with HMAC-SHA256.
type Signer struct {
	key []byte
}

// NewSigner returns a signer for the given secret (must be non-empty).
func NewSigner(secret []byte) *Signer {
	return &Signer{key: append([]byte(nil), secret...)}
}

const sigLen = 10

var b64 = base64.RawURLEncoding

func sourceByte(t string) (byte, bool) {
	switch t {
	case models.SourceJobItem:
		return 'j', true
	case models.SourceSend:
		return 's', true
	}
	return 0, false
}

func sourceName(b byte) (string, bool) {
	switch b {
	case 'j':
		return models.SourceJobItem, true
	case 's':
		return models.SourceSend, true
	}
	return "", false
}

// uuidBytes parses a canonical lowercase UUID string into 16 bytes.
func uuidBytes(id string) ([]byte, bool) {
	if len(id) != 36 || id[8] != '-' || id[13] != '-' || id[18] != '-' || id[23] != '-' {
		return nil, false
	}
	raw, err := hex.DecodeString(strings.ReplaceAll(id, "-", ""))
	if err != nil || len(raw) != 16 || formatUUID(raw) != id {
		return nil, false
	}
	return raw, true
}

func formatUUID(b []byte) string {
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

func (s *Signer) sign(payload []byte) []byte {
	m := hmac.New(sha256.New, s.key)
	m.Write(payload)
	return m.Sum(nil)[:sigLen]
}

// Encode returns a URL-safe token: base64url(payload) "." base64url(truncated HMAC).
// It returns "" when the payload has an unknown kind or source type.
func (s *Signer) Encode(p Payload) string {
	switch p.Kind {
	case KindOpen, KindClick, KindUnsubscribe:
	default:
		return ""
	}
	sb, ok := sourceByte(p.SourceType)
	if !ok {
		return ""
	}
	buf := []byte{byte(p.Kind), sb}
	if raw, ok := uuidBytes(p.SourceID); ok {
		buf = append(buf, 1)
		buf = append(buf, raw...)
	} else {
		buf = append(buf, 0)
		buf = binary.AppendUvarint(buf, uint64(len(p.SourceID)))
		buf = append(buf, p.SourceID...)
	}
	if p.Kind == KindClick {
		id := p.LinkID
		if id < 0 {
			id = 0
		}
		buf = binary.AppendUvarint(buf, uint64(id))
		buf = append(buf, p.URL...)
	}
	return b64.EncodeToString(buf) + "." + b64.EncodeToString(s.sign(buf))
}

// Decode verifies the signature and returns the payload, or ErrBadToken.
func (s *Signer) Decode(token string) (Payload, error) {
	var zero Payload
	if s == nil {
		return zero, ErrBadToken
	}
	dot := strings.IndexByte(token, '.')
	if dot < 0 || strings.IndexByte(token[dot+1:], '.') >= 0 {
		return zero, ErrBadToken
	}
	buf, err := b64.DecodeString(token[:dot])
	if err != nil {
		return zero, ErrBadToken
	}
	sig, err := b64.DecodeString(token[dot+1:])
	if err != nil || !hmac.Equal(sig, s.sign(buf)) {
		return zero, ErrBadToken
	}
	if len(buf) < 3 {
		return zero, ErrBadToken
	}
	p := Payload{Kind: Kind(buf[0])}
	switch p.Kind {
	case KindOpen, KindClick, KindUnsubscribe:
	default:
		return zero, ErrBadToken
	}
	st, ok := sourceName(buf[1])
	if !ok {
		return zero, ErrBadToken
	}
	p.SourceType = st
	rest := buf[3:]
	switch buf[2] {
	case 1:
		if len(rest) < 16 {
			return zero, ErrBadToken
		}
		p.SourceID = formatUUID(rest[:16])
		rest = rest[16:]
	case 0:
		n, k := binary.Uvarint(rest)
		if k <= 0 || n > uint64(len(rest)-k) {
			return zero, ErrBadToken
		}
		p.SourceID = string(rest[k : k+int(n)])
		rest = rest[k+int(n):]
	default:
		return zero, ErrBadToken
	}
	if p.Kind == KindClick {
		id, k := binary.Uvarint(rest)
		if k <= 0 || id > 1<<31 {
			return zero, ErrBadToken
		}
		p.LinkID = int(id)
		p.URL = string(rest[k:])
	} else if len(rest) != 0 {
		return zero, ErrBadToken
	}
	return p, nil
}
