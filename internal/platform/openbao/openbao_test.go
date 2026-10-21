package openbao_test

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/martialanouman/sms-platform/internal/platform/mtls"
	"github.com/martialanouman/sms-platform/internal/platform/mtls/mtlstest"
	"github.com/martialanouman/sms-platform/internal/platform/openbao"
)

func TestTheOpenBaoClientTrustsOnlyTheInternalCAOverTLS13(t *testing.T) {
	ca := mtlstest.New(t)
	files := ca.Issue(t, t.TempDir(), "localhost")
	client, err := openbao.HTTPClient(ca.PEM)
	if err != nil {
		t.Fatal(err)
	}
	server := func(files mtls.Files, maxVersion uint16) string {
		cert, err := tls.LoadX509KeyPair(files.Cert, files.Key)
		if err != nil {
			t.Fatal(err)
		}
		srv := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		srv.TLS = &tls.Config{Certificates: []tls.Certificate{cert}, MaxVersion: maxVersion}
		srv.StartTLS()
		t.Cleanup(srv.Close)
		_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
		return "https://localhost:" + port
	}
	get := func(url string) error {
		resp, err := client.Get(url)
		if err == nil {
			_ = resp.Body.Close()
		}
		return err
	}
	if err := get(server(files, 0)); err != nil {
		t.Fatalf("a server of the internal CA: %v", err)
	}
	if err := get(server(mtlstest.New(t).Issue(t, t.TempDir(), "localhost"), 0)); err == nil {
		t.Fatal("a server of another CA was trusted")
	}
	if err := get(server(files, tls.VersionTLS12)); err == nil {
		t.Fatal("a server capped at TLS 1.2 was accepted")
	}
	if _, err := openbao.HTTPClient([]byte("no certificate")); err == nil {
		t.Fatal("a CA file without certificate was accepted")
	}
}
