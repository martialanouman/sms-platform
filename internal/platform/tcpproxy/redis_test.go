package tcpproxy_test

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/martialanouman/sms-platform/internal/platform/tcpproxy"
)

func ping(addr string) error {
	c, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	_ = c.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := c.Write([]byte("PING\r\n")); err != nil {
		return err
	}
	line, err := bufio.NewReader(c).ReadString('\n')
	if err != nil {
		return err
	}
	if line != "+PONG\r\n" {
		return fmt.Errorf("unexpected reply %q", line)
	}
	return nil
}

func TestCuttingRedisIsSeenThroughTheProxyOnly(t *testing.T) {
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	redis, err := testcontainers.Run(ctx, "redis:8-alpine",
		testcontainers.WithExposedPorts("6379/tcp"),
		testcontainers.WithWaitStrategy(wait.ForListeningPort("6379/tcp")),
	)
	testcontainers.CleanupContainer(t, redis)
	if err != nil {
		t.Fatal(err)
	}
	direct, err := redis.PortEndpoint(ctx, "6379/tcp", "")
	if err != nil {
		t.Fatal(err)
	}
	p := tcpproxy.New(t, direct)
	if err := ping(p.Addr()); err != nil {
		t.Fatalf("through the proxy: %v", err)
	}
	p.Cut()
	if err := ping(direct); err != nil {
		t.Errorf("a direct client while cut: %v", err)
	}
	if err := ping(p.Addr()); err == nil {
		t.Error("a client crossed the cut proxy")
	}
	p.Resume()
	if err := ping(p.Addr()); err != nil {
		t.Errorf("after Resume: %v", err)
	}
}
