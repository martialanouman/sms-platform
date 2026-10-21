// Package openbao is the signer of the dashboard on OpenBao Transit, whose key never leaves OpenBao (spec 08 §8.4).
package openbao

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/martialanouman/sms-platform/internal/dashboard"
	"github.com/martialanouman/sms-platform/internal/platform/openbao"
)

const key = "dashboard-token"

// ErrUnknownKid is a kid that names no version of the signing key.
var ErrUnknownKid = errors.New("openbao: unknown kid")

// Signer signs under the Transit key dashboard-token; each version of the key is a kid.
type Signer struct {
	client *openbao.Client
}

// New returns the Signer of the OpenBao at addr, which logs in as role with the service account token in jwtFile.
func New(addr string, client *http.Client, role, jwtFile string) *Signer {
	return &Signer{client: openbao.New(addr, client, role, jwtFile)}
}

func kid(version int) string {
	return fmt.Sprintf("%s-v%d", key, version)
}

// Keys returns the kid of the latest version and the public key of every version Transit keeps.
func (s *Signer) Keys(ctx context.Context) (string, []dashboard.PublicKey, error) {
	var out struct {
		Data struct {
			LatestVersion int `json:"latest_version"`
			Keys          map[string]struct {
				PublicKey string `json:"public_key"`
			} `json:"keys"`
		} `json:"data"`
	}
	if err := s.client.Call(ctx, http.MethodGet, "transit/keys/"+key, nil, &out); err != nil {
		return "", nil, err
	}
	keys := make([]dashboard.PublicKey, 0, len(out.Data.Keys))
	for v, k := range out.Data.Keys {
		version, err := strconv.Atoi(v)
		if err != nil {
			return "", nil, fmt.Errorf("openbao: version %q: %w", v, err)
		}
		block, _ := pem.Decode([]byte(k.PublicKey))
		if block == nil {
			return "", nil, fmt.Errorf("openbao: version %d: no PEM public key", version)
		}
		parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
		public, ok := parsed.(*ecdsa.PublicKey)
		if err != nil || !ok {
			return "", nil, fmt.Errorf("openbao: version %d: not an ECDSA public key: %w", version, err)
		}
		keys = append(keys, dashboard.PublicKey{Kid: kid(version), Key: public})
	}
	if out.Data.LatestVersion == 0 || len(keys) == 0 {
		return "", nil, errors.New("openbao: no signing key")
	}
	return kid(out.Data.LatestVersion), keys, nil
}

// Sign returns the ES256 signature of input under the version of the key that kid names.
func (s *Signer) Sign(ctx context.Context, k string, input []byte) ([]byte, error) {
	version, err := strconv.Atoi(strings.TrimPrefix(k, key+"-v"))
	if err != nil {
		return nil, ErrUnknownKid
	}
	var out struct {
		Data struct {
			Signature string `json:"signature"`
		} `json:"data"`
	}
	in := map[string]any{"input": base64.StdEncoding.EncodeToString(input), "key_version": version,
		"hash_algorithm": "sha2-256", "marshaling_algorithm": "jws"}
	if err := s.client.Call(ctx, http.MethodPost, "transit/sign/"+key, in, &out); err != nil {
		return nil, err
	}
	sig, err := base64.RawURLEncoding.DecodeString(out.Data.Signature[strings.LastIndex(out.Data.Signature, ":")+1:])
	if err != nil || len(sig) != 64 {
		return nil, fmt.Errorf("openbao: transit/sign: no ES256 signature: %w", err)
	}
	return sig, nil
}
