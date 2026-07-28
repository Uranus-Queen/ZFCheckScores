package push

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"testing"
)

// TestEncryptRoundTrip locks the AES-256-GCM scheme used for the self-hosted
// page: encrypt in Go, decrypt with the exact same parameters a browser would
// use (SHA-256 key derivation, 12-byte nonce prefix, standard base64). If this
// breaks, the browser decrypter would silently fail with "解密失败".
func TestEncryptRoundTrip(t *testing.T) {
	plain := "<!DOCTYPE html><html><head><style>.x{color:red}</style></head>" +
		"<body><div>成绩: 92 分 · GPA 3.45</div></body></html>"
	key := "s3cr3t-key-123-🔑" // non-ASCII key must also work (UTF-8)

	ct, err := EncryptHTML(plain, key)
	if err != nil {
		t.Fatalf("EncryptHTML: %v", err)
	}
	// ciphertext must be pure base64 (safe to embed in JS string const)
	if ct == "" {
		t.Fatal("empty ciphertext")
	}
	raw, err := base64.StdEncoding.DecodeString(ct)
	if err != nil {
		t.Fatalf("ciphertext not valid base64: %v", err)
	}

	block, err := aes.NewCipher(deriveKey(key))
	if err != nil {
		t.Fatalf("aes.NewCipher: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("NewGCM: %v", err)
	}
	nonceSize := gcm.NonceSize()
	if len(raw) <= nonceSize {
		t.Fatal("ciphertext too short")
	}
	nonce, data := raw[:nonceSize], raw[nonceSize:]
	out, err := gcm.Open(nil, nonce, data, nil)
	if err != nil {
		t.Fatalf("gcm.Open: %v", err)
	}
	if string(out) != plain {
		t.Fatalf("round trip mismatch:\n got %q\nwant %q", string(out), plain)
	}
}

// TestEncryptKeyedByFragment verifies the key is derived from the secret, not
// the ciphertext, so a different key cannot decrypt (mirrors the browser
// rejecting a wrong #key with "解密失败").
func TestEncryptKeyedByFragment(t *testing.T) {
	plain := "secret grades"
	ct, _ := EncryptHTML(plain, "right-key")
	raw, _ := base64.StdEncoding.DecodeString(ct)

	block, _ := aes.NewCipher(deriveKey("wrong-key"))
	gcm, _ := cipher.NewGCM(block)
	nonceSize := gcm.NonceSize()
	if _, err := gcm.Open(nil, raw[:nonceSize], raw[nonceSize:], nil); err == nil {
		t.Fatal("wrong key should NOT decrypt, but it did")
	}
}
