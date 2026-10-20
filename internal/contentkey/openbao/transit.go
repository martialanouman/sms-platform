// Package openbao is the KMS of contentkey on OpenBao Transit, logged in by the Kubernetes auth method under the
// projected service account token of the pod (docs/steps/010a-brique-openbao.md, arbitrage 4).
package openbao

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/martialanouman/sms-platform/internal/contentkey"
)

// Transit encrypts and decrypts under the Transit keys its role's policy opens.
type Transit struct {
	addr, role, jwtFile string
	client              *http.Client

	sem   chan struct{}
	token string
	renew time.Time
}

// New returns the Transit of the OpenBao at addr, which logs in as role with the service account token in jwtFile.
func New(addr string, client *http.Client, role, jwtFile string) *Transit {
	return &Transit{addr: addr, role: role, jwtFile: jwtFile, client: client, sem: make(chan struct{}, 1)}
}

type statusError struct {
	path string
	code int
}

func (e statusError) Error() string {
	return fmt.Sprintf("openbao: %s: status %d", e.path, e.code)
}

// Encrypt returns the Transit ciphertext of plaintext under key.
func (t *Transit) Encrypt(ctx context.Context, key string, plaintext []byte) ([]byte, error) {
	var out struct {
		Data struct {
			Ciphertext string `json:"ciphertext"`
		} `json:"data"`
	}
	if err := t.call(ctx, "transit/encrypt/"+key, map[string]string{"plaintext": base64.StdEncoding.EncodeToString(plaintext)}, &out); err != nil {
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
	err := t.call(ctx, path, map[string]string{"ciphertext": string(sealed)}, &out)
	if s := (statusError{}); errors.As(err, &s) && s.path == path && s.code == http.StatusBadRequest {
		return nil, contentkey.ErrUndecryptable
	}
	if err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(out.Data.Plaintext)
}

func (t *Transit) call(ctx context.Context, path string, in, out any) error {
	token, err := t.login(ctx)
	if err != nil {
		return err
	}
	err = t.post(ctx, path, token, in, out)
	if s := (statusError{}); errors.As(err, &s) && s.code == http.StatusForbidden {
		if t.acquire(ctx) == nil {
			t.token = ""
			<-t.sem
		}
	}
	return err
}

func (t *Transit) login(ctx context.Context) (string, error) {
	if err := t.acquire(ctx); err != nil {
		return "", err
	}
	defer func() { <-t.sem }()
	if t.token != "" && time.Now().Before(t.renew) {
		return t.token, nil
	}
	jwt, err := os.ReadFile(t.jwtFile)
	if err != nil {
		return "", fmt.Errorf("openbao: %w", err)
	}
	var out struct {
		Auth struct {
			ClientToken   string `json:"client_token"`
			LeaseDuration int    `json:"lease_duration"`
		} `json:"auth"`
	}
	if err := t.post(ctx, "auth/kubernetes/login", "", map[string]string{"role": t.role, "jwt": string(jwt)}, &out); err != nil {
		return "", err
	}
	t.token, t.renew = out.Auth.ClientToken, time.Now().Add(time.Duration(out.Auth.LeaseDuration)*time.Second/2)
	return t.token, nil
}

func (t *Transit) acquire(ctx context.Context) error {
	select {
	case t.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (t *Transit) post(ctx context.Context, path, token string, in, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.addr+"/v1/"+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	if token != "" {
		req.Header.Set("X-Vault-Token", token)
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return statusError{path, resp.StatusCode}
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("openbao: %s: %w", path, err)
	}
	return nil
}
