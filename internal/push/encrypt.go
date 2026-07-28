package push

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"io"
)

// deriveKey expands a user-supplied key string into a 32-byte AES-256 key.
func deriveKey(keyStr string) []byte {
	sum := sha256.Sum256([]byte(keyStr))
	return sum[:]
}

// EncryptHTML encrypts plaintext with AES-256-GCM using a key derived from
// keyStr (SHA-256), returning base64(nonce || ciphertext). The output contains
// only base64 characters, so it is safe to embed in JSON / JS string constants
// (no quoting / escaping hazards). The browser counterpart lives in
// web/src/crypto.ts and must stay in exact sync with this scheme.
func EncryptHTML(plaintext, keyStr string) (string, error) {
	block, err := aes.NewCipher(deriveKey(keyStr))
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

