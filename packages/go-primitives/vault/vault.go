// Package vault is the ONLY place plaintext trading secrets may exist, and
// they never leave this process: callers get to USE a secret (build a signer,
// derive a client) but the raw bytes are never serialized. Storage is
// AES-256-GCM in Postgres BYTEA.
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
type Vault struct{ aead cipher.AEAD }

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
	return &Vault{aead: aead}, nil
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

// DecryptLegacy opens the TypeScript server's format: "iv:authTag:ciphertext"
// as hex strings in a TEXT column (lib/encryption.ts). Same key, so rows can
// be migrated in place to bytea without re-entering any secret.
func (v *Vault) DecryptLegacy(s string) (Secret, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return Secret{}, ErrBadCiphertext
	}
	iv, err1 := hex.DecodeString(parts[0])
	tag, err2 := hex.DecodeString(parts[1])
	ct, err3 := hex.DecodeString(parts[2])
	if err1 != nil || err2 != nil || err3 != nil || len(iv) != nonceSize {
		return Secret{}, ErrBadCiphertext
	}
	// Go's GCM wants ciphertext||tag concatenated.
	pt, err := v.aead.Open(nil, iv, append(ct, tag...), nil)
	if err != nil {
		return Secret{}, fmt.Errorf("vault: legacy: %w", err)
	}
	return Secret{v: pt}, nil
}
