package vault

import (
	"encoding/hex"
	"testing"
)

// Binary-layout blob produced by the earlier TypeScript runtime with the
// synthetic test key: proves the two implementations share one format.
func TestCrossRuntimeTSBlob(t *testing.T) {
	v, _ := New("000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f")
	blob, _ := hex.DecodeString("01e20cd65fbedd6e5a12d83af721c76a6e46cd06bfbb05bb60734b2ccd516bb57956ea0bb89459922462ee857d9fca140bcb8d5b")
	got, err := v.Decrypt(blob)
	if err != nil {
		t.Fatalf("go could not decrypt ts blob: %v", err)
	}
	if string(got.Reveal()) != "cross-runtime-secret-42" {
		t.Fatalf("plaintext mismatch: %q", got.Reveal())
	}
}
