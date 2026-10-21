// Package jwks reads the dashboard's JSON Web Key Set, reloading it on an unknown kid at most once a minute.
package jwks

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	reloadEvery = time.Minute
	timeout     = 5 * time.Second
	maxBody     = 64 << 10
)

var errUnknownKid = errors.New("jwks: unknown kid")

// Keys is the key set at URL, fetched by Client.
type Keys struct {
	URL      string
	Client   *http.Client
	Now      func() time.Time
	Log      *slog.Logger
	Failures prometheus.Counter

	mu     sync.Mutex
	keys   map[string]*ecdsa.PublicKey
	loaded time.Time
}

// Key is the public key of kid, reloading the set when kid is unknown and the last load is a minute old.
func (k *Keys) Key(ctx context.Context, kid string) (*ecdsa.PublicKey, error) {
	// ponytail: a cached lookup waits behind an in-flight reload (5 s at most); split the locks if that shows.
	k.mu.Lock()
	defer k.mu.Unlock()
	// debt: #163
	if key, ok := k.keys[kid]; ok {
		return key, nil
	}
	if k.Now().Sub(k.loaded) < reloadEvery {
		return nil, errUnknownKid
	}
	k.loaded = k.Now()
	keys, err := k.fetch(ctx)
	if err != nil {
		k.Failures.Inc()
		k.Log.WarnContext(ctx, "jwks not reloaded", "err", err)
		return nil, err
	}
	k.keys = keys
	if key, ok := keys[kid]; ok {
		return key, nil
	}
	return nil, errUnknownKid
}

func (k *Keys) fetch(ctx context.Context) (map[string]*ecdsa.PublicKey, error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, k.URL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := k.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jwks: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, err
	}
	var set struct {
		Keys []struct {
			Kid string `json:"kid"`
			X   string `json:"x"`
			Y   string `json:"y"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(body, &set); err != nil {
		return nil, err
	}
	keys := map[string]*ecdsa.PublicKey{}
	for _, jwk := range set.Keys {
		x, _ := base64.RawURLEncoding.DecodeString(jwk.X)
		y, _ := base64.RawURLEncoding.DecodeString(jwk.Y)
		key, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), slices.Concat([]byte{4}, x, y))
		if err != nil {
			continue
		}
		keys[jwk.Kid] = key
	}
	if len(keys) == 0 {
		return nil, errors.New("jwks: no key")
	}
	return keys, nil
}
