// Package vault seals stored credentials with AES-256-GCM in a versioned
// binary layout for a PostgreSQL BYTEA column, and opens them into a Secret
// whose raw bytes are never serialized: callers USE a secret (build a signer,
// derive a client) through an explicit Reveal.
package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

const (
	// formatV1 is the bytea layout: 0x01 || nonce(12) || ciphertext||tag.
	formatV1  = 0x01
	nonceSize = 12
	// legacyNonceSize is the IV length the earlier runtime's encrypt() used
	// for the text layout (crypto.randomBytes(16)). GCM accepts it, but only
	// through a cipher built for that nonce length.
	legacyNonceSize = 16
	gcmTagSize      = 16
)

var (
	ErrBadKey        = errors.New("vault: ENCRYPTION_KEY must be 64 hex chars (32 bytes)")
	ErrBadCiphertext = errors.New("vault: malformed ciphertext")
)

// Secret holds a decrypted value. Its String/Format/MarshalJSON all redact,
// so it cannot leak through logs or JSON encoders by accident — leaking
// requires calling Reveal() on purpose.
type Secret struct{ v []byte }

func (s Secret) String() string               { return "[redacted]" }
func (s Secret) GoString() string             { return "[redacted]" }
func (s Secret) MarshalJSON() ([]byte, error) { return []byte(`"[redacted]"`), nil }

// Reveal returns the plaintext. Call sites are the audit surface — grep for
// Reveal() to enumerate every place a secret is actually used.
func (s Secret) Reveal() []byte { return s.v }

// Vault encrypts/decrypts with a single 32-byte key.
type Vault struct {
	aead cipher.AEAD
	// legacy16 opens text-layout values written with a 16-byte IV. It is never
	// used to encrypt: every new write is the v1 bytea layout.
	legacy16 cipher.AEAD
}

func New(hexKey string) (*Vault, error) {
	key, err := hex.DecodeString(strings.TrimSpace(hexKey))
	if err != nil || len(key) != 32 {
		return nil, ErrBadKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	legacy16, err := cipher.NewGCMWithNonceSize(block, legacyNonceSize)
	if err != nil {
		return nil, err
	}
	return &Vault{aead: aead, legacy16: legacy16}, nil
}

// Encrypt seals plaintext into the v1 bytea layout.
func (v *Vault) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	out := make([]byte, 0, 1+nonceSize+len(plaintext)+v.aead.Overhead())
	out = append(out, formatV1)
	out = append(out, nonce...)
	return v.aead.Seal(out, nonce, plaintext, nil), nil
}

// Decrypt opens the v1 bytea layout.
func (v *Vault) Decrypt(blob []byte) (Secret, error) {
	if len(blob) < 1+nonceSize+v.aead.Overhead() || blob[0] != formatV1 {
		return Secret{}, ErrBadCiphertext
	}
	nonce := blob[1 : 1+nonceSize]
	pt, err := v.aead.Open(nil, nonce, blob[1+nonceSize:], nil)
	if err != nil {
		return Secret{}, fmt.Errorf("vault: %w", err)
	}
	return Secret{v: pt}, nil
}

// DecryptLegacy opens the earlier TypeScript runtime's text layout:
// "iv:authTag:ciphertext" as hex strings, under the same key.
//
// That layout was written with two IV lengths: 16 bytes by the earlier
// runtime's encrypt(), 12 by later writers. Both open here.
func (v *Vault) DecryptLegacy(s string) (Secret, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return Secret{}, ErrBadCiphertext
	}
	iv, err1 := hex.DecodeString(parts[0])
	tag, err2 := hex.DecodeString(parts[1])
	ct, err3 := hex.DecodeString(parts[2])
	if err1 != nil || err2 != nil || err3 != nil || len(tag) != gcmTagSize {
		return Secret{}, ErrBadCiphertext
	}
	var aead cipher.AEAD
	switch len(iv) {
	case nonceSize:
		aead = v.aead
	case legacyNonceSize:
		aead = v.legacy16
	default:
		return Secret{}, ErrBadCiphertext
	}
	// Go's GCM wants ciphertext||tag concatenated.
	pt, err := aead.Open(nil, iv, append(ct, tag...), nil)
	if err != nil {
		return Secret{}, fmt.Errorf("vault: legacy: %w", err)
	}
	return Secret{v: pt}, nil
}
