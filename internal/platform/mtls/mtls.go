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

// Files are the PEM paths of a service's certificate, its key and the internal CA.
type Files struct {
	Cert, Key, CA string
}

// Server returns the config of an internal server, which admits only the clients whose certificate carries a DNS SAN in
// allowed; nextProtos is the complete ALPN list. An empty allowlist is refused.
func (f Files) Server(allowed, nextProtos []string) (*tls.Config, error) {
	if len(allowed) == 0 {
		return nil, errors.New("mtls: an internal server names its callers; the allowlist is empty")
	}
	l, err := newLoader(f)
	if err != nil {
		return nil, err
	}
	allowed = slices.Clone(allowed)
	return &tls.Config{
		GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) {
			g := l.latest()
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
	l, err := newLoader(f)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		MinVersion: tls.VersionTLS13,
		RootCAs:    l.cur.pool,
		GetClientCertificate: func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
			return &l.latest().cert, nil
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
	cur   *generation
}

func newLoader(f Files) (*loader, error) {
	l := &loader{files: f}
	g, err := l.read()
	if err != nil {
		return nil, err
	}
	l.cur = g
	return l, nil
}

func (l *loader) latest() *generation {
	l.mu.Lock()
	defer l.mu.Unlock()
	if g, err := l.read(); err == nil {
		l.cur = g
	}
	return l.cur
}

func (l *loader) read() (*generation, error) {
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
	return &generation{cert, pool}, nil
}
