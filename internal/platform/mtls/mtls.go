// Package mtls builds the mutual TLS of the internal servers and clients from the internal CA (docs/design/06-confiance-interne.md).
package mtls

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"slices"
	"sync"
)

// Files are the PEM paths of a service's certificate, its key and the internal CA; they are reread when they change on disk.
type Files struct {
	Cert, Key, CA string
}

// Server returns the config of an internal server, which admits only the clients whose certificate carries a DNS SAN in
// allowed; nextProtos is the complete ALPN list. An empty allowlist is refused.
func (f Files) Server(allowed, nextProtos []string) (*tls.Config, error) {
	if len(allowed) == 0 {
		return nil, errors.New("mtls: an internal server names its callers; the allowlist is empty")
	}
	l := &loader{files: f}
	if _, err := l.load(); err != nil {
		return nil, err
	}
	allowed, nextProtos = slices.Clone(allowed), slices.Clone(nextProtos)
	return &tls.Config{
		GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) {
			g, err := l.load()
			if err != nil {
				return nil, err
			}
			return &tls.Config{
				MinVersion:       tls.VersionTLS13,
				NextProtos:       nextProtos,
				Certificates:     []tls.Certificate{g.cert},
				ClientCAs:        g.pool,
				ClientAuth:       tls.RequireAndVerifyClientCert,
				VerifyConnection: admit(allowed),
			}, nil
		},
	}, nil
}

// Client returns the config of an internal client; the caller sets ServerName to the bare service name it dials. The CA is
// read once: a CA rotation restarts the client.
func (f Files) Client() (*tls.Config, error) {
	l := &loader{files: f}
	g, err := l.load()
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		MinVersion: tls.VersionTLS13,
		RootCAs:    g.pool,
		GetClientCertificate: func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
			g, err := l.load()
			if err != nil {
				return nil, err
			}
			return &g.cert, nil
		},
	}, nil
}

func admit(allowed []string) func(tls.ConnectionState) error {
	return func(cs tls.ConnectionState) error {
		leaf := cs.VerifiedChains[0][0]
		if slices.ContainsFunc(leaf.DNSNames, func(san string) bool { return slices.Contains(allowed, san) }) {
			return nil
		}
		return fmt.Errorf("mtls: client %v is not an allowed caller %v", leaf.DNSNames, allowed)
	}
}

type generation struct {
	cert tls.Certificate
	pool *x509.CertPool
}

type loader struct {
	files Files
	mu    sync.Mutex
	stamp string
	cur   *generation
}

func (l *loader) load() (*generation, error) {
	var stamp string
	for _, p := range []string{l.files.Cert, l.files.Key, l.files.CA} {
		fi, err := os.Stat(p)
		if err != nil {
			return nil, fmt.Errorf("mtls: %w", err)
		}
		stamp += fmt.Sprintf("%d/%d;", fi.Size(), fi.ModTime().UnixNano())
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cur != nil && l.stamp == stamp {
		return l.cur, nil
	}
	cert, err := tls.LoadX509KeyPair(l.files.Cert, l.files.Key)
	if err != nil {
		return nil, fmt.Errorf("mtls: %w", err)
	}
	ca, err := os.ReadFile(l.files.CA)
	if err != nil {
		return nil, fmt.Errorf("mtls: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		return nil, fmt.Errorf("mtls: %s holds no certificate", l.files.CA)
	}
	l.cur, l.stamp = &generation{cert, pool}, stamp
	return l.cur, nil
}
