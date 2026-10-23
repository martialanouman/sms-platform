// Package contentkey seals the secrets admin-api-svc replays to a third party through content-key-svc (ADR-0016).
package contentkey

import (
	"context"
	"crypto/tls"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	contentkeyv1 "github.com/martialanouman/sms-platform/internal/gen/contentkey/v1"
)

const timeout = 5 * time.Second

// Client is admin-api-svc's connection to content-key-svc, which it may only ask to seal.
type Client struct {
	conn *grpc.ClientConn
	keys contentkeyv1.ConfigSecretsServiceClient
	log  *slog.Logger
}

// New dials addr lazily, as <service>:<port>; gRPC trusts only a server whose certificate names that service.
func New(addr string, tlsConfig *tls.Config, log *slog.Logger, opts ...grpc.DialOption) (*Client, error) {
	conn, err := grpc.NewClient(addr, append([]grpc.DialOption{grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig))}, opts...)...)
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn, keys: contentkeyv1.NewConfigSecretsServiceClient(conn), log: log}, nil
}

// Seal seals plaintext under the platform's configuration key; a failure is logged by its gRPC code alone.
func (c *Client) Seal(ctx context.Context, plaintext []byte) ([]byte, string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	resp, err := c.keys.Seal(ctx, &contentkeyv1.SealRequest{Plaintext: plaintext}, grpc.WaitForReady(true))
	if err != nil {
		c.log.ErrorContext(ctx, "content-key-svc did not seal", "code", status.Code(err).String())
		return nil, "", err
	}
	return resp.GetSealed(), resp.GetKmsKeyRef(), nil
}

// Close closes the connection.
func (c *Client) Close() error {
	return c.conn.Close()
}
