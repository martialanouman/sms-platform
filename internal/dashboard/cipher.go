package dashboard

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
)

const sealVersion = 1

// TotpKey seals the TOTP secrets at rest, AES-256-GCM, the operator as associated data (spec 08 §8.3).
type TotpKey struct {
	aead cipher.AEAD
}

// NewTotpKey wants exactly 32 bytes.
func NewTotpKey(key []byte) (TotpKey, error) {
	if len(key) != 32 {
		return TotpKey{}, errors.New("the TOTP key is not 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return TotpKey{}, err
	}
	aead, err := cipher.NewGCM(block)
	return TotpKey{aead: aead}, err
}

func (k TotpKey) seal(operatorID string, secret []byte) []byte {
	nonce := make([]byte, k.aead.NonceSize())
	_, _ = rand.Read(nonce)
	return k.aead.Seal(append([]byte{sealVersion}, nonce...), nonce, secret, []byte(operatorID))
}

func (k TotpKey) open(operatorID string, sealed []byte) ([]byte, error) {
	n := k.aead.NonceSize()
	if len(sealed) < 1+n || sealed[0] != sealVersion {
		return nil, errors.New("not a sealed TOTP secret")
	}
	return k.aead.Open(nil, sealed[1:1+n], sealed[1+n:], []byte(operatorID))
}
