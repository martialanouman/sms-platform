package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/martialanouman/sms-platform/internal/dashboard"
)

func (s server) IssueGatewayToken(ctx context.Context, request IssueGatewayTokenRequestObject) (IssueGatewayTokenResponseObject, error) {
	c, _ := ctx.Value(currentKey{}).(dashboard.Current)
	var requested []string
	if request.Body != nil && request.Body.Scopes != nil {
		requested = *request.Body.Scopes
	}
	issued, err := s.Service.IssuePersonalToken(ctx, c, requested)
	var refused dashboard.ForbiddenScope
	switch {
	case errors.As(err, &refused) && refused.Scope == "":
		return IssueGatewayToken403JSONResponse{Code: ErrorCodeForbiddenScope, Message: "Vos rôles n'ouvrent aucun scope qu'un jeton personnel puisse porter."}, nil
	case errors.As(err, &refused):
		return IssueGatewayToken403JSONResponse{Code: ErrorCodeForbiddenScope, Message: "Un jeton personnel ne peut pas porter " + refused.Scope + "."}, nil
	case errors.Is(err, dashboard.ErrSignerUnavailable):
		return IssueGatewayToken503JSONResponse{ServiceUnavailableJSONResponse{Code: ErrorCodeServiceUnavailable, Message: "La signature des jetons est indisponible ; réessayez dans un instant."}}, nil
	case err != nil:
		return nil, err
	}
	return IssueGatewayToken200JSONResponse{Token: issued.Token, Scopes: issued.Scopes, ExpiresAt: issued.ExpiresAt}, nil
}

// JWKS serves the public keys of the tokens at /.well-known/jwks.json, for the internal listener only (design 06 §7).
func JWKS(svc dashboard.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/.well-known/jwks.json" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		set, err := svc.JWKS(r.Context())
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "max-age=60")
		_, _ = w.Write(set)
	})
}
