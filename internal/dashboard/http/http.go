// Package http serves the dashboard BFF contract (contracts/openapi-bff.yaml) over the core's use cases.
package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	middleware "github.com/oapi-codegen/nethttp-middleware"

	"github.com/martialanouman/sms-platform/contracts"
	"github.com/martialanouman/sms-platform/internal/dashboard"
)

const (
	cookieName = "__Host-dashboard_session"
	maxBody    = 16 << 10
)

// Config is what the handler serves with.
type Config struct {
	Service        dashboard.Service
	Origin         string
	TrustedProxies []netip.Prefix
	Log            *slog.Logger
	Assets         fs.FS
}

// New serves the contract under /api and the SPA everywhere else.
func New(c Config) (http.Handler, error) {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	loader.ReadFromURIFunc = func(_ *openapi3.Loader, u *url.URL) ([]byte, error) {
		return fs.ReadFile(contracts.OpenAPI(), path.Clean(u.Path))
	}
	spec, err := loader.LoadFromURI(&url.URL{Path: "openapi-bff.yaml"})
	if err != nil {
		return nil, err
	}
	spec.Servers = nil
	required, auth := permissions(spec)
	s := server{Config: c, permission: required, auth: auth}
	strict := NewStrictHandlerWithOptions(s, []StrictMiddlewareFunc{s.session, exchange}, StrictHTTPServerOptions{
		RequestErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, _ error) {
			write(w, http.StatusUnprocessableEntity, ErrorCodeValidationError, "La requête est mal formée.", nil)
		},
		ResponseErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			c.Log.ErrorContext(r.Context(), "dashboard request failed", "path", r.URL.Path, "error", err)
			write(w, http.StatusInternalServerError, ErrorCodeInternalError, "Erreur interne.", nil)
		},
	})
	validate := middleware.OapiRequestValidatorWithOptions(spec, &middleware.Options{
		Options:              openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc},
		ErrorHandlerWithOpts: refuse,
		Prefix:               "/api",
	})
	mux := http.NewServeMux()
	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		write(w, http.StatusNotFound, ErrorCodeNotFound, "Aucune ressource à cette adresse.", nil)
	})
	root := http.NewServeMux()
	root.Handle("/api/", noStore(c.guard(validate(HandlerWithOptions(strict, StdHTTPServerOptions{BaseURL: "/api", BaseRouter: mux})))))
	root.Handle("/", spa(c.Assets))
	return secured(root), nil
}

func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func secured(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		next.ServeHTTP(w, r)
	})
}

func spa(assets fs.FS) http.Handler {
	files := http.FileServerFS(assets)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if name := strings.TrimPrefix(r.URL.Path, "/"); strings.HasPrefix(name, "assets/") {
			if info, err := fs.Stat(assets, name); err != nil || info.IsDir() {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			files.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFileFS(w, r, assets, "index.html")
	})
}

func permissions(spec *openapi3.T) (required map[string][]string, auth map[string]bool) {
	required, auth = map[string][]string{}, map[string]bool{}
	for path, item := range spec.Paths.Map() {
		for _, op := range item.Operations() {
			name := strings.ToUpper(op.OperationID[:1]) + op.OperationID[1:]
			auth[name] = strings.HasPrefix(path, "/auth/")
			if op.Security == nil {
				continue
			}
			for _, requirement := range *op.Security {
				if scopes, ok := requirement["session"]; ok {
					required[name] = scopes
				}
			}
		}
	}
	return required, auth
}

func (c Config) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			if r.Header.Get("Origin") != c.Origin {
				write(w, http.StatusForbidden, ErrorCodeForbiddenOrigin, "La requête ne vient pas du tableau de bord.", nil)
				return
			}
			if media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || media != "application/json" {
				write(w, http.StatusUnsupportedMediaType, ErrorCodeUnsupportedMediaType, "La requête doit être en application/json.", nil)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		}
		next.ServeHTTP(w, r)
	})
}

func refuse(_ context.Context, err error, w http.ResponseWriter, _ *http.Request, opts middleware.ErrorHandlerOpts) {
	var tooLarge *http.MaxBytesError
	var request *openapi3filter.RequestError
	switch {
	case errors.As(err, &tooLarge):
		write(w, http.StatusRequestEntityTooLarge, ErrorCodePayloadTooLarge, "La requête est trop volumineuse.", nil)
	case opts.MatchedRoute == nil:
		write(w, http.StatusNotFound, ErrorCodeNotFound, "Aucune ressource à cette adresse.", nil)
	case errors.As(err, &request):
		placed := leaves(err)
		if request.Parameter != nil {
			placed = []fieldError{{Field: request.Parameter.Name, Message: "valeur refusée"}}
		}
		if len(placed) == 0 {
			placed = []fieldError{{Field: "body", Message: "corps illisible"}}
		}
		write(w, http.StatusUnprocessableEntity, ErrorCodeValidationError, "La requête est mal formée.", placed)
	default:
		write(w, http.StatusInternalServerError, ErrorCodeInternalError, "Erreur interne.", nil)
	}
}

var located = regexp.MustCompile(`^error at "/([^"]+)": (?:at '[^']*': )?(.+)$`)

func leaves(err error) []fieldError {
	var schema *openapi3.SchemaError
	if !errors.As(err, &schema) {
		return nil
	}
	var multi openapi3.MultiError
	if schema.Origin != nil && errors.As(schema.Origin, &multi) {
		var all []fieldError
		for _, e := range multi {
			all = append(all, leaves(e)...)
		}
		return all
	}
	if m := located.FindStringSubmatch(schema.Reason); m != nil {
		return []fieldError{{Field: strings.ReplaceAll(m[1], "/", "."), Message: m[2]}}
	}
	return nil
}

type fieldError = struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func write(w http.ResponseWriter, status int, code ErrorCode, message string, fields []fieldError) {
	body := Error{Code: code, Message: message}
	if fields != nil {
		body.Errors = &fields
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

type exchangeKey struct{}

type exchanged struct {
	w http.ResponseWriter
	r *http.Request
}

func exchange(next StrictHandlerFunc, _ string) StrictHandlerFunc {
	return func(ctx context.Context, w http.ResponseWriter, r *http.Request, request any) (any, error) {
		return next(context.WithValue(ctx, exchangeKey{}, exchanged{w, r}), w, r, request)
	}
}

func from(ctx context.Context) exchanged {
	e, _ := ctx.Value(exchangeKey{}).(exchanged)
	return e
}

type currentKey struct{}

func (s server) session(next StrictHandlerFunc, operation string) StrictHandlerFunc {
	required, protected := s.permission[operation]
	if !protected {
		return next
	}
	unauthenticated := "Aucune session ouverte."
	if operation == "VerifyMfa" {
		unauthenticated = factorRefused
	}
	return func(ctx context.Context, w http.ResponseWriter, r *http.Request, request any) (any, error) {
		cookie, _ := r.Cookie(cookieName)
		var value string
		if cookie != nil {
			value = cookie.Value
		}
		current, err := s.Service.Resolve(ctx, value)
		switch {
		case errors.Is(err, dashboard.ErrUnauthenticated):
			write(w, http.StatusUnauthorized, ErrorCodeUnauthenticated, unauthenticated, nil)
			return nil, nil
		case err != nil:
			return nil, err
		}
		if !s.auth[operation] && !current.Elevated && dashboard.RequiresMFA(current.Permissions) {
			write(w, http.StatusForbidden, ErrorCodeMfaRequired, "Vérifiez d'abord votre second facteur.", nil)
			return nil, nil
		}
		for _, permission := range required {
			if !slices.Contains(current.Permissions, permission) {
				write(w, http.StatusForbidden, ErrorCodeForbiddenScope, fmt.Sprintf("Il faut la permission %s.", permission), nil)
				return nil, nil
			}
		}
		return next(context.WithValue(ctx, currentKey{}, current), w, r, request)
	}
}

func (c Config) source(r *http.Request) netip.Addr {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	addr, _ := netip.ParseAddr(host)
	trusted := func(a netip.Addr) bool {
		return slices.ContainsFunc(c.TrustedProxies, func(p netip.Prefix) bool { return p.Contains(a.Unmap()) })
	}
	hops := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(hops) - 1; i >= 0 && trusted(addr); i-- {
		hop, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			break
		}
		addr = hop
	}
	return addr
}

func sessionCookie(value string, maxAge int) *http.Cookie {
	return &http.Cookie{Name: cookieName, Value: value, Path: "/", MaxAge: maxAge, HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode}
}
