package mtls_test

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/martialanouman/sms-platform/internal/platform/mtls"
	"github.com/martialanouman/sms-platform/internal/platform/mtls/mtlstest"
)

func write(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
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

func pair(t *testing.T, ca mtlstest.Authority, serverName, clientName string, allowed ...string) (*tls.Config, *tls.Config, mtls.Files, mtls.Files) {
	t.Helper()
	sf, cf := ca.Issue(t, t.TempDir(), serverName), ca.Issue(t, t.TempDir(), clientName)
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
	ca := mtlstest.New(t)
	server, err := ca.Issue(t, t.TempDir(), "admin-api-svc").Server([]string{"billing-svc", "dashboard"}, []string{"h2", "http/1.1"})
	if err != nil {
		t.Fatal(err)
	}
	client, err := ca.Issue(t, t.TempDir(), "dashboard-canary", "dashboard").Client()
	if err != nil {
		t.Fatal(err)
	}
	client.ServerName = "admin-api-svc"
	client.NextProtos = []string{"h2"}
	ss, cs, serr, cerr := handshake(t, server, client)
	if serr != nil || cerr != nil {
		t.Fatalf("handshake: server %v, client %v", serr, cerr)
	}
	if got := ss.PeerCertificates[0].DNSNames; len(got) != 2 || got[1] != "dashboard" {
		t.Errorf("server saw %v", got)
	}
	if cs.Version != tls.VersionTLS13 || cs.NegotiatedProtocol != "h2" {
		t.Errorf("version %x, protocol %q", cs.Version, cs.NegotiatedProtocol)
	}
}

func TestAnUnnamedCallerIsRefused(t *testing.T) {
	ca := mtlstest.New(t)
	for name, c := range map[string]struct {
		setup func(*testing.T, *tls.Config)
		cause string
	}{
		"other SAN": {func(*testing.T, *tls.Config) {}, "not an allowed caller"},
		"no certificate": {func(_ *testing.T, c *tls.Config) {
			c.GetClientCertificate = func(*tls.CertificateRequestInfo) (*tls.Certificate, error) { return &tls.Certificate{}, nil }
		}, "didn't provide a certificate"},
		"other authority": {func(t *testing.T, c *tls.Config) {
			other, err := mtlstest.New(t).Issue(t, t.TempDir(), "dashboard").Client()
			if err != nil {
				t.Fatal(err)
			}
			c.GetClientCertificate = other.GetClientCertificate
		}, "unknown authority"},
	} {
		t.Run(name, func(t *testing.T) {
			server, client, _, _ := pair(t, ca, "admin-api-svc", "router-svc", "dashboard", "cn-router-svc")
			c.setup(t, client)
			if _, _, serr, _ := handshake(t, server, client); serr == nil || !strings.Contains(serr.Error(), c.cause) {
				t.Errorf("server error = %v, want %q", serr, c.cause)
			}
		})
	}
}

func TestTheAllowlistIsCopied(t *testing.T) {
	ca := mtlstest.New(t)
	allowed := []string{"dashboard"}
	server, err := ca.Issue(t, t.TempDir(), "admin-api-svc").Server(allowed, nil)
	if err != nil {
		t.Fatal(err)
	}
	allowed[0] = "router-svc"
	client, err := ca.Issue(t, t.TempDir(), "router-svc").Client()
	if err != nil {
		t.Fatal(err)
	}
	client.ServerName = "admin-api-svc"
	if _, _, serr, _ := handshake(t, server, client); serr == nil || !strings.Contains(serr.Error(), "not an allowed caller") {
		t.Errorf("server error = %v: a caller added to the slice after construction was admitted", serr)
	}
}

func TestTheClientRefusesTLS12(t *testing.T) {
	ca := mtlstest.New(t)
	sf := ca.Issue(t, t.TempDir(), "admin-api-svc")
	cert, err := tls.LoadX509KeyPair(sf.Cert, sf.Key)
	if err != nil {
		t.Fatal(err)
	}
	client, err := ca.Issue(t, t.TempDir(), "dashboard").Client()
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
	f := mtlstest.New(t).Issue(t, t.TempDir(), "admin-api-svc")
	for _, allowed := range [][]string{nil, {}} {
		if _, err := f.Server(allowed, nil); err == nil {
			t.Errorf("Server(%v) built a config that admits the whole authority", allowed)
		}
	}
}

func TestUnreadableFilesFailAtConstruction(t *testing.T) {
	good := mtlstest.New(t).Issue(t, t.TempDir(), "admin-api-svc")
	notPEM := filepath.Join(t.TempDir(), "ca.crt")
	if err := os.WriteFile(notPEM, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, f := range map[string]mtls.Files{
		"missing CA":  {Cert: good.Cert, Key: good.Key, CA: "/nonexistent"},
		"missing key": {Cert: good.Cert, Key: "/nonexistent", CA: good.CA},
		"empty CA":    {Cert: good.Cert, Key: good.Key, CA: notPEM},
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
	server, client, _, _ := pair(t, mtlstest.New(t), "admin-api-svc", "dashboard", "dashboard")
	client.MinVersion, client.MaxVersion = tls.VersionTLS12, tls.VersionTLS12
	if _, _, serr, _ := handshake(t, server, client); serr == nil || !strings.Contains(serr.Error(), "version") {
		t.Errorf("server error = %v, want a protocol version refusal", serr)
	}
}

func TestRenewedCertificatesAreServedWithoutRestart(t *testing.T) {
	ca := mtlstest.New(t)
	server, client, sf, cf := pair(t, ca, "admin-api-svc", "dashboard", "dashboard")
	ss, cs, serr, cerr := handshake(t, server, client)
	if serr != nil || cerr != nil {
		t.Fatalf("first handshake: %v, %v", serr, cerr)
	}
	serverSerial, clientSerial := cs.PeerCertificates[0].SerialNumber, ss.PeerCertificates[0].SerialNumber

	ca.Issue(t, filepath.Dir(sf.Cert), "admin-api-svc")
	ca.Issue(t, filepath.Dir(cf.Cert), "dashboard")
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

func TestAFailedRereadServesThePreviousGeneration(t *testing.T) {
	server, client, sf, cf := pair(t, mtlstest.New(t), "admin-api-svc", "dashboard", "dashboard")
	ss, cs, serr, cerr := handshake(t, server, client)
	if serr != nil || cerr != nil {
		t.Fatalf("first handshake: %v, %v", serr, cerr)
	}
	write(t, sf.Cert, []byte("half-written"))
	write(t, cf.Cert, []byte("half-written"))
	ss2, cs2, serr, cerr := handshake(t, server, client)
	if serr != nil || cerr != nil {
		t.Fatalf("handshake during a broken rotation: %v, %v", serr, cerr)
	}
	if cs2.PeerCertificates[0].SerialNumber.Cmp(cs.PeerCertificates[0].SerialNumber) != 0 ||
		ss2.PeerCertificates[0].SerialNumber.Cmp(ss.PeerCertificates[0].SerialNumber) != 0 {
		t.Error("the previous generation was not served")
	}
}

func TestAnAddedAuthorityIsAdmittedWithoutRestart(t *testing.T) {
	ca, next := mtlstest.New(t), mtlstest.New(t)
	server, _, sf, _ := pair(t, ca, "admin-api-svc", "dashboard", "dashboard")
	client, err := next.Issue(t, t.TempDir(), "dashboard").Client()
	if err != nil {
		t.Fatal(err)
	}
	client.RootCAs.AppendCertsFromPEM(ca.PEM)
	client.ServerName = "admin-api-svc"
	if _, _, serr, _ := handshake(t, server, client); serr == nil || !strings.Contains(serr.Error(), "unknown authority") {
		t.Fatalf("before the rotation: server error = %v, want an unknown authority", serr)
	}
	write(t, sf.CA, append(append([]byte{}, ca.PEM...), next.PEM...))
	if _, _, serr, cerr := handshake(t, server, client); serr != nil || cerr != nil {
		t.Errorf("after the rotation: server %v, client %v", serr, cerr)
	}
}

func TestARemovedAuthorityIsRefusedWithoutRestart(t *testing.T) {
	ca, next := mtlstest.New(t), mtlstest.New(t)
	server, _, sf, _ := pair(t, ca, "admin-api-svc", "dashboard", "dashboard")
	write(t, sf.CA, append(append([]byte{}, ca.PEM...), next.PEM...))
	client, err := next.Issue(t, t.TempDir(), "dashboard").Client()
	if err != nil {
		t.Fatal(err)
	}
	client.RootCAs.AppendCertsFromPEM(ca.PEM)
	client.ServerName = "admin-api-svc"
	client.ClientSessionCache = tls.NewLRUClientSessionCache(1)
	if _, _, serr, cerr := handshake(t, server, client); serr != nil || cerr != nil {
		t.Fatalf("first handshake: %v, %v", serr, cerr)
	}
	if _, cs, serr, cerr := handshake(t, server, client); serr != nil || cerr != nil || !cs.DidResume {
		t.Fatalf("resumed handshake: %v, %v, resumed %v", serr, cerr, cs.DidResume)
	}
	write(t, sf.CA, ca.PEM)
	if _, _, serr, _ := handshake(t, server, client); serr == nil || !strings.Contains(serr.Error(), "unknown authority") {
		t.Errorf("after the removal: server error = %v, want an unknown authority", serr)
	}
}

func TestConcurrentHandshakesDuringARotation(t *testing.T) {
	ca := mtlstest.New(t)
	server, client, sf, _ := pair(t, ca, "admin-api-svc", "dashboard", "dashboard")
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, serr, cerr := handshake(t, server, client.Clone()); serr != nil || cerr != nil {
				errs <- fmt.Errorf("server %v, client %v", serr, cerr)
			}
		}()
	}
	ca.Issue(t, filepath.Dir(sf.Cert), "admin-api-svc")
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
