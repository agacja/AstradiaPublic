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

// Vector produced by the earlier TypeScript runtime with testKey:
//
//	node -e 'crypto.createCipheriv("aes-256-gcm", key, iv) ...'
//
// Ensures the older text layout opens.
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

// The text-layout vectors of the shared fixture
// (packages/platform-core/tests/fixtures/vault.json, same testKey): one per IV
// length the layout was written with.
func TestLegacyCompatBothIVLengths(t *testing.T) {
	const plaintext = "cross-runtime-secret-42"
	v, err := New(testKey)
	if err != nil {
		t.Fatal(err)
	}
	for name, legacy := range map[string]string{
		"12-byte IV": "424242424242424242424242:83ae3fe086425af563edde2d25fec0ed:79ee16b5fea8d7997606475e18b17c38513887b2cba813",
		"16-byte IV": "42424242424242424242424242424242:cc8237997139d3eaeaee6ba1c2b2e00f:1f0e4c28cf2818cfb11c6078e35ecd18463c7ced1e9a23",
	} {
		got, err := v.DecryptLegacy(legacy)
		if err != nil {
			t.Fatalf("%s: legacy decrypt: %v", name, err)
		}
		if string(got.Reveal()) != plaintext {
			t.Fatalf("%s: plaintext mismatch: %q", name, got.Reveal())
		}
	}
}

func TestLegacyRejectsMalformedWithoutPanicking(t *testing.T) {
	v, _ := New(testKey)
	const (
		iv16 = "42424242424242424242424242424242"
		tag  = "cc8237997139d3eaeaee6ba1c2b2e00f"
		ct   = "1f0e4c28cf2818cfb11c6078e35ecd18463c7ced1e9a23"
	)
	for name, legacy := range map[string]string{
		"empty":             "",
		"two parts":         iv16 + ":" + tag,
		"four parts":        iv16 + ":" + tag + ":" + ct + ":00",
		"not hex":           "zz:" + tag + ":" + ct,
		"8-byte IV":         "4242424242424242:" + tag + ":" + ct,
		"14-byte IV":        "4242424242424242424242424242:" + tag + ":" + ct,
		"short tag":         iv16 + ":cc8237997139d3eaeaee6ba1:" + ct,
		"tampered tag":      iv16 + ":dc8237997139d3eaeaee6ba1c2b2e00f:" + ct,
		"tampered body":     iv16 + ":" + tag + ":0f0e4c28cf2818cfb11c6078e35ecd18463c7ced1e9a23",
		"12-byte IV prefix": iv16[:24] + ":" + tag + ":" + ct,
	} {
		if _, err := v.DecryptLegacy(legacy); err == nil {
			t.Errorf("%s: accepted", name)
		}
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
