package vault

import (
	"bytes"
	"encoding/json"
	"testing"
)

const testKey = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"

func TestRoundTrip(t *testing.T) {
	v, err := New(testKey)
	if err != nil {
		t.Fatal(err)
	}
	blob, err := v.Encrypt([]byte("0xdeadbeefprivatekey"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := v.Decrypt(blob)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Reveal(), []byte("0xdeadbeefprivatekey")) {
		t.Fatalf("round trip mismatch")
	}
}

func TestTamperDetected(t *testing.T) {
	v, _ := New(testKey)
	blob, _ := v.Encrypt([]byte("secret"))
	blob[len(blob)-1] ^= 0xff
	if _, err := v.Decrypt(blob); err == nil {
		t.Fatal("tampered ciphertext accepted")
	}
}

// Vector produced by the TypeScript server's lib/encryption.ts with testKey:
//
//	node -e 'crypto.createCipheriv("aes-256-gcm", key, iv) ...'
//
// Ensures in-place migration from the TEXT column works against real rows.
func TestLegacyCompat(t *testing.T) {
	v, _ := New(testKey)
	legacy := "000000000000000000000001:dae72518c560f6c59693217c15cbaeac:7db3d3902bd447717c423518" // "hello world!"
	got, err := v.DecryptLegacy(legacy)
	if err != nil {
		t.Fatalf("legacy decrypt: %v", err)
	}
	if string(got.Reveal()) != "hello world!" {
		t.Fatalf("legacy plaintext mismatch: %q", got.Reveal())
	}
}

func TestSecretNeverSerializes(t *testing.T) {
	v, _ := New(testKey)
	blob, _ := v.Encrypt([]byte("api-key"))
	s, _ := v.Decrypt(blob)

	if s.String() != "[redacted]" {
		t.Fatal("String leaks")
	}
	j, _ := json.Marshal(struct{ K Secret }{s})
	if !bytes.Contains(j, []byte("[redacted]")) || bytes.Contains(j, []byte("api-key")) {
		t.Fatalf("json leaks: %s", j)
	}
}
