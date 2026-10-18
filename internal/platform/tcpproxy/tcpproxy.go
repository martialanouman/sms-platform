// Package tcpproxy puts a TCP proxy that a test can cut in front of a dependency (.claude/rules/tests.md).
package tcpproxy

import (
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

// Proxy relays every connection to its upstream until it is cut.
type Proxy struct {
	ln       net.Listener
	upstream string

	mu      sync.Mutex
	cut     bool
	clients map[net.Conn]struct{}
	wg      sync.WaitGroup
}

// New starts a proxy to upstream on a local port; it is closed when the test ends.
func New(t testing.TB, upstream string) *Proxy {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("tcpproxy: %v", err)
	}
	p := &Proxy{ln: ln, upstream: upstream, clients: map[net.Conn]struct{}{}}
	p.wg.Go(p.accept)
	t.Cleanup(p.Close)
	return p
}

// Addr is the address clients dial instead of the upstream.
func (p *Proxy) Addr() string {
	return p.ln.Addr().String()
}

// Cut closes every live connection and drops every new one once accepted, until Resume.
func (p *Proxy) Cut() {
	p.mu.Lock()
	p.cut = true
	live := make([]net.Conn, 0, len(p.clients))
	for c := range p.clients {
		live = append(live, c)
	}
	p.mu.Unlock()
	for _, c := range live {
		_ = c.Close()
	}
}

// Resume relays new connections again.
func (p *Proxy) Resume() {
	p.mu.Lock()
	p.cut = false
	p.mu.Unlock()
}

// Close stops accepting, closes every connection and waits for the relays to end.
func (p *Proxy) Close() {
	_ = p.ln.Close()
	p.Cut()
	p.wg.Wait()
}

func (p *Proxy) accept() {
	for {
		client, err := p.ln.Accept()
		if err != nil {
			return
		}
		p.mu.Lock()
		if p.cut {
			p.mu.Unlock()
			_ = client.Close()
			continue
		}
		p.clients[client] = struct{}{}
		p.mu.Unlock()
		p.wg.Go(func() { p.relay(client) })
	}
}

func (p *Proxy) relay(client net.Conn) {
	defer func() { _ = client.Close() }()
	upstream, err := net.DialTimeout("tcp", p.upstream, 3*time.Second)
	if err != nil {
		return
	}
	defer func() { _ = upstream.Close() }()
	done := make(chan struct{}, 2)
	pump := func(dst, src net.Conn) {
		_, _ = io.Copy(dst, src)
		done <- struct{}{}
	}
	go pump(upstream, client)
	go pump(client, upstream)
	<-done
}
