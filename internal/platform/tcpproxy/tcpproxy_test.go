package tcpproxy_test

import (
	"bufio"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/martialanouman/sms-platform/internal/platform/tcpproxy"
)

func echo(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = c.Close() }()
				_, _ = io.Copy(c, c)
			}()
		}
	}()
	return ln.Addr().String()
}

func dial(t *testing.T, addr string) net.Conn {
	t.Helper()
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	_ = c.SetDeadline(time.Now().Add(2 * time.Second))
	return c
}

func roundTrip(c net.Conn) error {
	if _, err := c.Write([]byte("ping\n")); err != nil {
		return err
	}
	line, err := bufio.NewReader(c).ReadString('\n')
	if err != nil {
		return err
	}
	if line != "ping\n" {
		return errors.New("echoed " + line)
	}
	return nil
}

func TestBytesCrossTheProxy(t *testing.T) {
	p := tcpproxy.New(t, echo(t))
	if err := roundTrip(dial(t, p.Addr())); err != nil {
		t.Fatal(err)
	}
}

func TestCutDropsLiveAndNewConnectionsUntilResume(t *testing.T) {
	p := tcpproxy.New(t, echo(t))
	live := dial(t, p.Addr())
	if err := roundTrip(live); err != nil {
		t.Fatal(err)
	}
	p.Cut()
	if _, err := live.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Errorf("read = %v: a live connection survived Cut", err)
	}
	if err := roundTrip(dial(t, p.Addr())); err == nil {
		t.Error("a new connection crossed a cut proxy")
	}
	p.Resume()
	if err := roundTrip(dial(t, p.Addr())); err != nil {
		t.Errorf("after Resume: %v", err)
	}
}

func TestCloseReleasesEverything(t *testing.T) {
	p := tcpproxy.New(t, echo(t))
	live := dial(t, p.Addr())
	if err := roundTrip(live); err != nil {
		t.Fatal(err)
	}
	closed := make(chan struct{})
	go func() { p.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not return with a live connection")
	}
	if _, err := live.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Errorf("read = %v: a live connection survived Close", err)
	}
	if c, err := net.Dial("tcp", p.Addr()); err == nil {
		_ = c.Close()
		t.Error("the proxy still accepts after Close")
	}
}

func TestAnUnreachableUpstreamDropsTheClient(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	gone := ln.Addr().String()
	_ = ln.Close()
	p := tcpproxy.New(t, gone)
	if _, err := dial(t, p.Addr()).Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Errorf("read = %v, want the client closed with no upstream", err)
	}
}

func TestTheProxyClosesWithItsTest(t *testing.T) {
	upstream := echo(t)
	var addr string
	t.Run("user", func(t *testing.T) { addr = tcpproxy.New(t, upstream).Addr() })
	if c, err := net.Dial("tcp", addr); err == nil {
		_ = c.Close()
		t.Error("the proxy outlived its test")
	}
}

func TestEachSideClosesTheOther(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	ended := make(chan error, 1)
	go func() {
		first, err := ln.Accept()
		if err != nil {
			return
		}
		_ = first.Close()
		second, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = second.Close() }()
		_ = second.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, err = second.Read(make([]byte, 1))
		ended <- err
	}()
	p := tcpproxy.New(t, ln.Addr().String())

	if _, err := dial(t, p.Addr()).Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Errorf("upstream closed: client read = %v, want EOF", err)
	}
	_ = dial(t, p.Addr()).Close()
	if err := <-ended; !errors.Is(err, io.EOF) {
		t.Errorf("client closed: upstream read = %v, want EOF", err)
	}
}
