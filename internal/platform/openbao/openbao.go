// Package openbao is the client of OpenBao, logged in by the Kubernetes auth method under the projected service account
// token of the pod (docs/steps/010a-brique-openbao.md, arbitrage 4).
package openbao

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"
)

// Client calls OpenBao under the token of its Kubernetes role, logged in once and renewed past half its lease.
type Client struct {
	addr, role, jwtFile string
	client              *http.Client

	sem   chan struct{}
	token string
	renew time.Time
}

// New returns the Client of the OpenBao at addr, which logs in as role with the service account token in jwtFile; it
// calls through a copy of client that follows no redirect, which would carry the token and the JWT elsewhere.
func New(addr string, client *http.Client, role, jwtFile string) *Client {
	c := *client
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{addr: addr, role: role, jwtFile: jwtFile, client: &c, sem: make(chan struct{}, 1)}
}

// HTTPClient is the client of an OpenBao served under the internal CA ca, over TLS 1.3 and without client certificate.
func HTTPClient(ca []byte) (*http.Client, error) {
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		return nil, errors.New("the internal CA holds no certificate")
	}
	return &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots}}}, nil
}

// StatusError is an answer of OpenBao other than 200; it carries neither the body nor the token.
type StatusError struct {
	Path string
	Code int
}

func (e StatusError) Error() string {
	return fmt.Sprintf("openbao: %s: status %d", e.Path, e.Code)
}

// Call sends method to path with in as JSON body, when not nil, and decodes the answer into out. A 403 forgets the
// token, so that the next call logs in again.
func (c *Client) Call(ctx context.Context, method, path string, in, out any) error {
	token, err := c.login(ctx)
	if err != nil {
		return err
	}
	err = c.do(ctx, method, path, token, in, out)
	if s := (StatusError{}); errors.As(err, &s) && s.Code == http.StatusForbidden {
		if c.acquire(ctx) == nil {
			c.token = ""
			<-c.sem
		}
	}
	return err
}

func (c *Client) login(ctx context.Context) (string, error) {
	if err := c.acquire(ctx); err != nil {
		return "", err
	}
	defer func() { <-c.sem }()
	if c.token != "" && time.Now().Before(c.renew) {
		return c.token, nil
	}
	jwt, err := os.ReadFile(c.jwtFile)
	if err != nil {
		return "", fmt.Errorf("openbao: %w", err)
	}
	var out struct {
		Auth struct {
			ClientToken   string `json:"client_token"`
			LeaseDuration int    `json:"lease_duration"`
		} `json:"auth"`
	}
	if err := c.do(ctx, http.MethodPost, "auth/kubernetes/login", "", map[string]string{"role": c.role, "jwt": string(jwt)}, &out); err != nil {
		return "", err
	}
	c.token, c.renew = out.Auth.ClientToken, time.Now().Add(time.Duration(out.Auth.LeaseDuration)*time.Second/2)
	return c.token, nil
}

func (c *Client) acquire(ctx context.Context) error {
	select {
	case c.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Client) do(ctx context.Context, method, path, token string, in, out any) error {
	var body bytes.Reader
	if in != nil {
		data, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body.Reset(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.addr+"/v1/"+path, &body)
	if err != nil {
		return err
	}
	if token != "" {
		req.Header.Set("X-Vault-Token", token)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return StatusError{path, resp.StatusCode}
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("openbao: %s: %w", path, err)
	}
	return nil
}
