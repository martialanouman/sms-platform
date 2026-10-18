package mtls_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/martialanouman/sms-platform/internal/platform/mtls"
)

var issued int

type authority struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	pem  []byte
}

func newAuthority(t *testing.T) authority {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "internal-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return authority{cert, key, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}
}

func (a authority) issue(t *testing.T, dir, name string) mtls.Files {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "cn-" + name},
		DNSNames:     []string{name},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, a.cert, &key.PublicKey, a.key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	f := mtls.Files{Cert: filepath.Join(dir, "tls.crt"), Key: filepath.Join(dir, "tls.key"), CA: filepath.Join(dir, "ca.crt")}
	for path, data := range map[string][]byte{
		f.Cert: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		f.Key:  pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}),
		f.CA:   a.pem,
	} {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		issued++
		later := time.Now().Add(time.Duration(issued) * time.Minute)
		if err := os.Chtimes(path, later, later); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func handshake(t *testing.T, server, client *tls.Config) (tls.ConnectionState, tls.ConnectionState, error, error) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	type result struct {
		state tls.ConnectionState
		err   error
	}
	done := make(chan result, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			done <- result{err: err}
			return
		}
		defer func() { _ = conn.Close() }()
		srv := tls.Server(conn, server)
		_ = srv.SetDeadline(time.Now().Add(5 * time.Second))
		err = srv.Handshake()
		done <- result{srv.ConnectionState(), err}
	}()
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	cli := tls.Client(conn, client)
	_ = cli.SetDeadline(time.Now().Add(5 * time.Second))
	cerr := cli.Handshake()
	if cerr == nil {
		_ = cli.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		_, cerr = cli.Read(make([]byte, 1))
		if ne, ok := cerr.(net.Error); errors.Is(cerr, io.EOF) || ok && ne.Timeout() {
			cerr = nil
		}
	}
	r := <-done
	return r.state, cli.ConnectionState(), r.err, cerr
}

func pair(t *testing.T, ca authority, serverName, clientName string, allowed ...string) (*tls.Config, *tls.Config, mtls.Files, mtls.Files) {
	t.Helper()
	sf, cf := ca.issue(t, t.TempDir(), serverName), ca.issue(t, t.TempDir(), clientName)
	server, err := sf.Server(allowed, []string{"h2", "http/1.1"})
	if err != nil {
		t.Fatal(err)
	}
	client, err := cf.Client()
	if err != nil {
		t.Fatal(err)
	}
	client.ServerName = serverName
	return server, client, sf, cf
}

func TestANamedCallerIsAdmitted(t *testing.T) {
	server, client, _, _ := pair(t, newAuthority(t), "admin-api-svc", "dashboard", "billing-svc", "dashboard")
	client.NextProtos = []string{"h2"}
	ss, cs, serr, cerr := handshake(t, server, client)
	if serr != nil || cerr != nil {
		t.Fatalf("handshake: server %v, client %v", serr, cerr)
	}
	if got := ss.PeerCertificates[0].DNSNames; len(got) != 1 || got[0] != "dashboard" {
		t.Errorf("server saw %v", got)
	}
	if cs.Version != tls.VersionTLS13 || cs.NegotiatedProtocol != "h2" {
		t.Errorf("version %x, protocol %q", cs.Version, cs.NegotiatedProtocol)
	}
}

func TestAnUnnamedCallerIsRefused(t *testing.T) {
	ca := newAuthority(t)
	for name, setup := range map[string]func(*tls.Config){
		"other SAN": func(*tls.Config) {},
		"no certificate": func(c *tls.Config) {
			c.GetClientCertificate = func(*tls.CertificateRequestInfo) (*tls.Certificate, error) { return &tls.Certificate{}, nil }
		},
		"other authority": func(c *tls.Config) {
			other, err := newAuthority(t).issue(t, t.TempDir(), "dashboard").Client()
			if err != nil {
				t.Fatal(err)
			}
			c.GetClientCertificate = other.GetClientCertificate
		},
	} {
		t.Run(name, func(t *testing.T) {
			server, client, _, _ := pair(t, ca, "admin-api-svc", "router-svc", "dashboard", "cn-router-svc")
			setup(client)
			if _, _, serr, _ := handshake(t, server, client); serr == nil {
				t.Error("the server admitted the caller")
			}
		})
	}
}

func TestTheAllowlistIsCopied(t *testing.T) {
	ca := newAuthority(t)
	allowed := []string{"dashboard"}
	server, err := ca.issue(t, t.TempDir(), "admin-api-svc").Server(allowed, nil)
	if err != nil {
		t.Fatal(err)
	}
	allowed[0] = "router-svc"
	client, err := ca.issue(t, t.TempDir(), "router-svc").Client()
	if err != nil {
		t.Fatal(err)
	}
	client.ServerName = "admin-api-svc"
	if _, _, serr, _ := handshake(t, server, client); serr == nil {
		t.Error("a caller added to the slice after construction was admitted")
	}
}

func TestTheClientRefusesTLS12(t *testing.T) {
	ca := newAuthority(t)
	sf := ca.issue(t, t.TempDir(), "admin-api-svc")
	cert, err := tls.LoadX509KeyPair(sf.Cert, sf.Key)
	if err != nil {
		t.Fatal(err)
	}
	client, err := ca.issue(t, t.TempDir(), "dashboard").Client()
	if err != nil {
		t.Fatal(err)
	}
	client.ServerName = "admin-api-svc"
	server := &tls.Config{Certificates: []tls.Certificate{cert}, MaxVersion: tls.VersionTLS12}
	if _, _, _, cerr := handshake(t, server, client); cerr == nil || !strings.Contains(cerr.Error(), "version") {
		t.Errorf("client error = %v, want a protocol version refusal", cerr)
	}
}

func TestServerRefusesAnEmptyAllowlist(t *testing.T) {
	f := newAuthority(t).issue(t, t.TempDir(), "admin-api-svc")
	for _, allowed := range [][]string{nil, {}} {
		if _, err := f.Server(allowed, nil); err == nil {
			t.Errorf("Server(%v) built a config that admits the whole authority", allowed)
		}
	}
}

func TestUnreadableFilesFailAtConstruction(t *testing.T) {
	good := newAuthority(t).issue(t, t.TempDir(), "admin-api-svc")
	notPEM := filepath.Join(t.TempDir(), "ca.crt")
	if err := os.WriteFile(notPEM, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, f := range map[string]mtls.Files{
		"missing cert": {Cert: "/nonexistent", Key: good.Key, CA: good.CA},
		"missing CA":   {Cert: good.Cert, Key: good.Key, CA: "/nonexistent"},
		"empty CA":     {Cert: good.Cert, Key: good.Key, CA: notPEM},
	} {
		if _, err := f.Server([]string{"dashboard"}, nil); err == nil {
			t.Errorf("%s: Server built a config", name)
		}
		if _, err := f.Client(); err == nil {
			t.Errorf("%s: Client built a config", name)
		}
	}
}

func TestTLS12IsRefused(t *testing.T) {
	server, client, _, _ := pair(t, newAuthority(t), "admin-api-svc", "dashboard", "dashboard")
	client.MinVersion, client.MaxVersion = tls.VersionTLS12, tls.VersionTLS12
	if _, _, serr, _ := handshake(t, server, client); serr == nil || !strings.Contains(serr.Error(), "version") {
		t.Errorf("server error = %v, want a protocol version refusal", serr)
	}
}

func TestRenewedCertificatesAreServedWithoutRestart(t *testing.T) {
	ca := newAuthority(t)
	server, client, sf, cf := pair(t, ca, "admin-api-svc", "dashboard", "dashboard")
	ss, cs, serr, cerr := handshake(t, server, client)
	if serr != nil || cerr != nil {
		t.Fatalf("first handshake: %v, %v", serr, cerr)
	}
	serverSerial, clientSerial := cs.PeerCertificates[0].SerialNumber, ss.PeerCertificates[0].SerialNumber

	ca.issue(t, filepath.Dir(sf.Cert), "admin-api-svc")
	ca.issue(t, filepath.Dir(cf.Cert), "dashboard")
	ss, cs, serr, cerr = handshake(t, server, client)
	if serr != nil || cerr != nil {
		t.Fatalf("second handshake: %v, %v", serr, cerr)
	}
	if cs.PeerCertificates[0].SerialNumber.Cmp(serverSerial) == 0 {
		t.Error("the server still serves its old certificate")
	}
	if ss.PeerCertificates[0].SerialNumber.Cmp(clientSerial) == 0 {
		t.Error("the client still presents its old certificate")
	}
}
