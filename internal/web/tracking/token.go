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
	"time"

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

// ErrExpiredToken is returned for a correctly signed token older than MaxAge
var ErrExpiredToken = errors.New("tracking: token expired")

// Token lifetimes counted from the issue time. A click token is a signed
// redirect to an arbitrary URL, so it must not live forever; unsubscribe
// links have to keep working for old mail much longer.
const (
	MaxAgeOpenClick   = 180 * 24 * time.Hour
	MaxAgeUnsubscribe = 2 * 365 * 24 * time.Hour
)

// MaxAge returns the lifetime of tokens of kind k.
func MaxAge(k Kind) time.Duration {
	if k == KindUnsubscribe {
		return MaxAgeUnsubscribe
	}
	return MaxAgeOpenClick
}

// Payload is what a token carries. SourceType is models.SourceJobItem or models.SourceSend.
type Payload struct {
	Kind       Kind
	SourceType string
	SourceID   string
	LinkID     int    // click only: data-sl number
	URL        string // click only: final destination URL
	IssuedAt   int64  // unix seconds; Encode fills the current time when 0
	// RecipientID is recipients.id, unsubscribe only and optional. The
	// recipient row outlives the job item (cleanup and campaign deletion
	// remove items), so the link keeps working after the item is gone.
	RecipientID string
}

// Signer encodes and verifies tokens with HMAC-SHA256. Tokens are signed
// with the first key; any key verifies, so after a secret rotation links in
// mail signed with the previous secret keep working.
type Signer struct {
	keys [][]byte
	now  func() time.Time
}

// NewSigner returns a signer for the given secret (must be non-empty).
// previous are older secrets still accepted by Decode; empty ones are skipped.
func NewSigner(secret []byte, previous ...[]byte) *Signer {
	s := &Signer{keys: [][]byte{append([]byte(nil), secret...)}, now: time.Now}
	for _, p := range previous {
		if len(p) > 0 {
			s.keys = append(s.keys, append([]byte(nil), p...))
		}
	}
	return s
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

// appendID appends an id: flag 1 and 16 bytes for a canonical UUID, otherwise
// flag 0, uvarint length and the bytes.
func appendID(buf []byte, id string) []byte {
	if raw, ok := uuidBytes(id); ok {
		buf = append(buf, 1)
		return append(buf, raw...)
	}
	buf = append(buf, 0)
	buf = binary.AppendUvarint(buf, uint64(len(id)))
	return append(buf, id...)
}

// readID reads an id written by appendID and returns it with the rest of buf.
func readID(buf []byte) (string, []byte, bool) {
	if len(buf) < 1 {
		return "", nil, false
	}
	rest := buf[1:]
	switch buf[0] {
	case 1:
		if len(rest) < 16 {
			return "", nil, false
		}
		return formatUUID(rest[:16]), rest[16:], true
	case 0:
		n, k := binary.Uvarint(rest)
		if k <= 0 || n > uint64(len(rest)-k) {
			return "", nil, false
		}
		return string(rest[k : k+int(n)]), rest[k+int(n):], true
	}
	return "", nil, false
}

func sign(key, payload []byte) []byte {
	m := hmac.New(sha256.New, key)
	m.Write(payload)
	return m.Sum(nil)[:sigLen]
}

func (s *Signer) sign(payload []byte) []byte { return sign(s.keys[0], payload) }

// verify reports whether sig matches payload under any of the keys.
func (s *Signer) verify(payload, sig []byte) bool {
	for _, k := range s.keys {
		if hmac.Equal(sig, sign(k, payload)) {
			return true
		}
	}
	return false
}

// Encode returns a URL-safe token: base64url(payload) "." base64url(truncated HMAC).
// The payload is kind, source type, issue time (uvarint unix seconds), source
// id and, for clicks, link id and URL; for unsubscribes, the optional
// recipient id.
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
	iat := p.IssuedAt
	if iat <= 0 {
		iat = s.now().Unix()
	}
	buf := []byte{byte(p.Kind), sb}
	buf = binary.AppendUvarint(buf, uint64(iat))
	buf = appendID(buf, p.SourceID)
	if p.Kind == KindUnsubscribe && p.RecipientID != "" {
		buf = appendID(buf, p.RecipientID)
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

// Decode verifies the signature and returns the payload, ErrBadToken or,
// for a token older than MaxAge of its kind, ErrExpiredToken.
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
	if err != nil || !s.verify(buf, sig) {
		return zero, ErrBadToken
	}
	if len(buf) < 4 {
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
	iat, k := binary.Uvarint(buf[2:])
	if k <= 0 || iat == 0 || iat > 1<<40 || len(buf) < 2+k+1 {
		return zero, ErrBadToken
	}
	p.IssuedAt = int64(iat)
	var rest []byte
	if p.SourceID, rest, ok = readID(buf[2+k:]); !ok {
		return zero, ErrBadToken
	}
	// Unsubscribe tokens issued before the recipient id was added end here
	if p.Kind == KindUnsubscribe && len(rest) > 0 {
		if p.RecipientID, rest, ok = readID(rest); !ok {
			return zero, ErrBadToken
		}
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
	if s.now().Sub(time.Unix(p.IssuedAt, 0)) > MaxAge(p.Kind) {
		return zero, ErrExpiredToken
	}
	return p, nil
}
