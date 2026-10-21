// Package openbao is the KMS of contentkey on OpenBao Transit.
package openbao

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"

	"github.com/martialanouman/sms-platform/internal/contentkey"
	"github.com/martialanouman/sms-platform/internal/platform/openbao"
)

// Transit encrypts and decrypts under the Transit keys its role's policy opens.
type Transit struct {
	client *openbao.Client
}

// New returns the Transit of the OpenBao at addr, which logs in as role with the service account token in jwtFile.
func New(addr string, client *http.Client, role, jwtFile string) *Transit {
	return &Transit{client: openbao.New(addr, client, role, jwtFile)}
}

// Encrypt returns the Transit ciphertext of plaintext under key.
func (t *Transit) Encrypt(ctx context.Context, key string, plaintext []byte) ([]byte, error) {
	var out struct {
		Data struct {
			Ciphertext string `json:"ciphertext"`
		} `json:"data"`
	}
	if err := t.client.Call(ctx, http.MethodPost, "transit/encrypt/"+key, map[string]string{"plaintext": base64.StdEncoding.EncodeToString(plaintext)}, &out); err != nil {
		return nil, err
	}
	if out.Data.Ciphertext == "" {
		return nil, errors.New("openbao: transit/encrypt: no ciphertext")
	}
	return []byte(out.Data.Ciphertext), nil
}

// Decrypt reverses Encrypt; a ciphertext Transit refuses is contentkey.ErrUndecryptable.
func (t *Transit) Decrypt(ctx context.Context, key string, sealed []byte) ([]byte, error) {
	var out struct {
		Data struct {
			Plaintext string `json:"plaintext"`
		} `json:"data"`
	}
	path := "transit/decrypt/" + key
	err := t.client.Call(ctx, http.MethodPost, path, map[string]string{"ciphertext": string(sealed)}, &out)
	if s := (openbao.StatusError{}); errors.As(err, &s) && s.Path == path && s.Code == http.StatusBadRequest {
		return nil, contentkey.ErrUndecryptable
	}
	if err != nil {
		return nil, err
	}
	if out.Data.Plaintext == "" {
		return nil, errors.New("openbao: transit/decrypt: no plaintext")
	}
	return base64.StdEncoding.DecodeString(out.Data.Plaintext)
}
