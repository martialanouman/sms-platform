// Package openbaotest runs a dev OpenBao in a container, whose Kubernetes auth method trusts a fake token review.
package openbaotest

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/martialanouman/sms-platform/internal/platform/mtls/mtlstest"
)

const root = "root"

// Step is an administrative write, made under the root token before the test.
type Step struct {
	Path string
	Body any
}

// Bao is a running OpenBao: Addr is its URL, JWT the file of the service account token, Reviews the logins reviewed.
type Bao struct {
	Addr    string
	JWT     string
	Reviews atomic.Int32
	account string
}

// Start runs OpenBao with Transit mounted and the Kubernetes role account, bound to the service account account of
// namespace sms under the policy account, whose tokens live ttl; setup then runs.
func Start(t *testing.T, account, ttl string, setup ...Step) *Bao {
	t.Helper()
	b := &Bao{JWT: filepath.Join(t.TempDir(), "token"), account: account}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	review := http.NewServeMux()
	review.HandleFunc("POST /apis/authentication.k8s.io/v1/tokenreviews", func(w http.ResponseWriter, r *http.Request) {
		b.Reviews.Add(1)
		var in struct {
			Spec struct {
				Token string `json:"token"`
			} `json:"spec"`
		}
		current, err := os.ReadFile(b.JWT)
		if json.NewDecoder(r.Body).Decode(&in) != nil || err != nil || in.Spec.Token != string(current) {
			_, _ = fmt.Fprint(w, `{"apiVersion":"authentication.k8s.io/v1","kind":"TokenReview","status":{"authenticated":false}}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"apiVersion":"authentication.k8s.io/v1","kind":"TokenReview","status":{"authenticated":true,`+
			`"user":{"username":"system:serviceaccount:sms:`+account+`","uid":"sa-uid"},"audiences":["openbao"]}}`)
	})
	srv := &http.Server{Handler: review, ReadHeaderTimeout: time.Second}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	ctr, err := testcontainers.Run(t.Context(), "openbao/openbao:2.7.1",
		testcontainers.WithCmd("server", "-dev", "-dev-root-token-id="+root, "-dev-listen-address=0.0.0.0:8200"),
		testcontainers.WithExposedPorts("8200/tcp"),
		testcontainers.WithHostPortAccess(port),
		testcontainers.WithWaitStrategy(wait.ForHTTP("/v1/sys/health").WithPort("8200/tcp")),
	)
	testcontainers.CleanupContainer(t, ctr)
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := ctr.PortEndpoint(t.Context(), "8200/tcp", "http")
	if err != nil {
		t.Fatal(err)
	}
	b.Addr = endpoint
	t.Cleanup(func() {
		if t.Failed() {
			r, _ := ctr.Logs(context.Background())
			data, _ := io.ReadAll(r)
			t.Logf("openbao logs:\n%s", data)
		}
	})
	for _, step := range append([]Step{
		{"sys/mounts/transit", map[string]any{"type": "transit"}},
		{"sys/auth/kubernetes", map[string]any{"type": "kubernetes"}},
		{"auth/kubernetes/config", map[string]any{"kubernetes_host": fmt.Sprintf("http://%s:%d", testcontainers.HostInternal, port),
			"disable_local_ca_jwt": true, "kubernetes_ca_cert": string(mtlstest.New(t).PEM)}},
		{"auth/kubernetes/role/" + account, map[string]any{"bound_service_account_names": account,
			"bound_service_account_namespaces": "sms", "audience": "openbao", "token_ttl": ttl, "token_max_ttl": ttl,
			"token_policies": account, "token_no_default_policy": true}},
	}, setup...) {
		b.Admin(t, step.Path, step.Body)
	}
	b.Rotate(t)
	return b
}

// Rotate writes a new service account token, as the kubelet does.
func (b *Bao) Rotate(t *testing.T) {
	t.Helper()
	if err := os.WriteFile(b.JWT, []byte(serviceAccountJWT(t, b.account)), 0o600); err != nil {
		t.Fatal(err)
	}
}

// Admin posts body to path under the root token and returns the answer.
func (b *Bao) Admin(t *testing.T, path string, body any) []byte {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, b.Addr+"/v1/"+path, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Vault-Token", root)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(resp.Body)
		t.Fatalf("%s: status %d: %s", path, resp.StatusCode, msg)
	}
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func serviceAccountJWT(t *testing.T, account string) string {
	t.Helper()
	enc := func(v any) string {
		data, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return base64.RawURLEncoding.EncodeToString(data)
	}
	now := time.Now().Unix()
	unsigned := enc(map[string]string{"alg": "ES256", "typ": "JWT"}) + "." + enc(map[string]any{
		"iss": "https://kubernetes.default.svc", "sub": "system:serviceaccount:sms:" + account, "aud": []string{"openbao"},
		"iat": now, "nbf": now, "exp": now + 3600,
		"kubernetes.io": map[string]any{"namespace": "sms", "serviceaccount": map[string]string{"name": account, "uid": "sa-uid"}},
	})
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(unsigned))
	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(append(r.FillBytes(make([]byte, 32)), s.FillBytes(make([]byte, 32))...))
}
