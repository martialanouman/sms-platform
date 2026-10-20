// Package grpc serves contentkey over gRPC, behind an interceptor that authorizes each caller method by method: the mTLS
// allowlist admits a binary at the handshake and never sees the method (ADR-0016).
package grpc

import (
	"context"
	"errors"
	"slices"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"github.com/martialanouman/sms-platform/internal/contentkey"
	contentkeyv1 "github.com/martialanouman/sms-platform/internal/gen/contentkey/v1"
)

// Callers maps each caller, the DNS SAN of its certificate, to the full methods it may call; any other call is denied.
var Callers = map[string][]string{
	"admin-api-svc": {contentkeyv1.ConfigSecretsService_Seal_FullMethodName},
}

// Authorize is the unary interceptor that lets through only the calls Callers grants.
func Authorize(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	p, ok := peer.FromContext(ctx)
	if ok {
		if tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo); ok && len(tlsInfo.State.VerifiedChains) > 0 {
			for _, san := range tlsInfo.State.VerifiedChains[0][0].DNSNames {
				if slices.Contains(Callers[san], info.FullMethod) {
					return handler(ctx, req)
				}
			}
		}
	}
	return nil, status.Error(codes.PermissionDenied, "caller not allowed on this method")
}

// Register registers the ConfigSecrets service of cs on s.
func Register(s grpc.ServiceRegistrar, cs contentkey.ConfigSecrets) {
	contentkeyv1.RegisterConfigSecretsServiceServer(s, configSecrets{cs: cs})
}

type configSecrets struct {
	contentkeyv1.UnimplementedConfigSecretsServiceServer
	cs contentkey.ConfigSecrets
}

func (c configSecrets) Seal(ctx context.Context, req *contentkeyv1.SealRequest) (*contentkeyv1.SealResponse, error) {
	sealed, ref, err := c.cs.Seal(ctx, req.GetPlaintext())
	if err != nil {
		return nil, verdict(err)
	}
	return &contentkeyv1.SealResponse{Sealed: sealed, KmsKeyRef: ref}, nil
}

func (c configSecrets) Open(ctx context.Context, req *contentkeyv1.OpenRequest) (*contentkeyv1.OpenResponse, error) {
	plaintext, err := c.cs.Open(ctx, req.GetKmsKeyRef(), req.GetSealed())
	if err != nil {
		return nil, verdict(err)
	}
	return &contentkeyv1.OpenResponse{Plaintext: plaintext}, nil
}

func verdict(err error) error {
	switch {
	case errors.Is(err, contentkey.ErrEmptySecret):
		return status.Error(codes.InvalidArgument, "empty secret")
	case errors.Is(err, contentkey.ErrUnknownKey):
		return status.Error(codes.InvalidArgument, "unknown kms key ref")
	case errors.Is(err, contentkey.ErrUndecryptable):
		return status.Error(codes.DataLoss, "sealed secret refused by the KMS")
	default:
		return status.Error(codes.Unavailable, "KMS unavailable")
	}
}
