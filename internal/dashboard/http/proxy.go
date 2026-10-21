package http

import (
	"errors"
	"io/fs"
	"log/slog"
	"maps"
	"mime"
	"net/http"
	"net/http/httputil"
	"slices"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/routers/gorillamux"

	"github.com/martialanouman/sms-platform/contracts"
	"github.com/martialanouman/sms-platform/internal/dashboard"
)

const relayPrefix = "/api/gw"

// proxy relays the operations of the Admin API contract to the gateway under the operator's token, deciding nothing
// the gateway decides (spec 08 §8.2, I12, I14).
func (s server) proxy() (http.Handler, error) {
	raw, err := fs.ReadFile(contracts.OpenAPI(), "openapi-admin.yaml")
	if err != nil {
		return nil, err
	}
	spec, err := openapi3.NewLoader().LoadFromData(raw)
	if err != nil {
		return nil, err
	}
	spec.Servers = nil
	router, err := gorillamux.NewRouter(spec)
	if err != nil {
		return nil, err
	}
	relay := &httputil.ReverseProxy{
		Rewrite:   func(r *httputil.ProxyRequest) { r.SetURL(s.Gateway) },
		Transport: s.GatewayTransport,
		ErrorLog:  slog.NewLogLogger(s.Log.Handler(), slog.LevelWarn),
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			s.Log.WarnContext(r.Context(), "admin api unreachable", "operation", r.URL.Path, "error", err)
			unavailable(w)
		},
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		out := r.Clone(r.Context())
		out.URL.Path = strings.TrimPrefix(r.URL.Path, relayPrefix)
		route, _, err := router.FindRoute(out)
		if err != nil || route.Operation.Extensions["x-websocket-message"] != nil {
			write(w, http.StatusNotFound, ErrorCodeNotFound, "Aucune ressource à cette adresse.", nil)
			return
		}
		if unsafe(r) {
			if r.Header.Get("Origin") != s.Origin {
				write(w, http.StatusForbidden, ErrorCodeForbiddenOrigin, "La requête ne vient pas du tableau de bord.", nil)
				return
			}
			media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || !slices.Contains(accepted(route.Operation), media) {
				write(w, http.StatusUnsupportedMediaType, ErrorCodeUnsupportedMediaType, "Ce type de contenu n'est pas accepté ici.", nil)
				return
			}
		}
		current, err := s.current(r.Context(), r)
		switch {
		case errors.Is(err, dashboard.ErrUnauthenticated):
			write(w, http.StatusUnauthorized, ErrorCodeUnauthenticated, "Aucune session ouverte.", nil)
			return
		case err != nil:
			s.Log.ErrorContext(r.Context(), "session not resolved", "error", err)
			write(w, http.StatusInternalServerError, ErrorCodeInternalError, "Erreur interne.", nil)
			return
		}
		if !current.Elevated && dashboard.RequiresMFA(current.Permissions) {
			write(w, http.StatusForbidden, ErrorCodeMfaRequired, "Vérifiez d'abord votre second facteur.", nil)
			return
		}
		token, err := s.Service.IssueOperatorToken(r.Context(), current)
		if err != nil {
			s.Log.WarnContext(r.Context(), "operator token not signed", "error", err)
			unavailable(w)
			return
		}
		out.Header.Del("Cookie")
		out.Header.Set("Authorization", "Bearer "+token)
		relay.ServeHTTP(w, out)
	}), nil
}

func accepted(op *openapi3.Operation) []string {
	if op.RequestBody == nil || op.RequestBody.Value == nil {
		return []string{"application/json"}
	}
	return slices.Collect(maps.Keys(op.RequestBody.Value.Content))
}

func unavailable(w http.ResponseWriter) {
	write(w, http.StatusServiceUnavailable, ErrorCodeServiceUnavailable, "La passerelle ne répond pas.", nil)
}
