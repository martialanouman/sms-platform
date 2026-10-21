// Package huma serves the Admin API contract (contracts/openapi-admin.yaml) over the core of admin-api-svc.
package huma

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/google/uuid"

	"github.com/martialanouman/sms-platform/internal/adminapi"
	errs "github.com/martialanouman/sms-platform/internal/platform/errors"
)

const scheme = "OperatorBearer"

// Config is what the Admin API serves with.
type Config struct {
	Store    adminapi.Store
	Verifier adminapi.Verifier
}

// New is the handler of the Admin API, and its huma API, whose OpenAPI document the contract guard reads.
func New(c Config) (http.Handler, huma.API) {
	mux := http.NewServeMux()
	cfg := huma.DefaultConfig("SMS Gateway — Admin API", "1.0.0")
	cfg.OpenAPIPath, cfg.DocsPath, cfg.SchemasPath, cfg.CreateHooks = "", "", "", nil
	cfg.RejectUnknownQueryParameters = true
	cfg.Components.SecuritySchemes = map[string]*huma.SecurityScheme{scheme: {Type: "http", Scheme: "bearer", BearerFormat: "JWT"}}
	api := humago.New(mux, cfg)
	api.UseMiddleware(authorize(api, c))
	registerAudit(api)
	return audit{mux: mux}, api
}

type state struct {
	requestID string
	failed    bool
	response  *buffer
}

type stateKey struct{}

// ponytail: every response is buffered until its audit line commits; a stream (026) needs to pass through.
type audit struct {
	mux http.Handler
}

func (a audit) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := uuid.Must(uuid.NewV7())
	buf := &buffer{header: http.Header{}}
	st := &state{requestID: id.String(), response: buf}
	a.mux.ServeHTTP(buf, r.WithContext(context.WithValue(r.Context(), stateKey{}, st)))
	w.Header().Set("X-Request-Id", st.requestID)
	if st.failed {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"code":"service_unavailable","message":"service unavailable"}`))
		return
	}
	for k, v := range buf.header {
		w.Header()[k] = v
	}
	w.WriteHeader(buf.status)
	_, _ = w.Write(buf.body.Bytes())
}

type buffer struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (b *buffer) Header() http.Header { return b.header }

func (b *buffer) WriteHeader(status int) {
	if b.status == 0 {
		b.status = status
	}
}

func (b *buffer) Write(p []byte) (int, error) {
	b.WriteHeader(http.StatusOK)
	return b.body.Write(p)
}

var errRefused = errors.New("refused by its handler")

func authorize(api huma.API, c Config) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		st, _ := ctx.Context().Value(stateKey{}).(*state)
		op := ctx.Operation()
		var scope string
		if len(op.Security) == 1 && len(op.Security[0][scheme]) == 1 {
			scope = op.Security[0][scheme][0]
		}
		bearer, found := strings.CutPrefix(ctx.Header("Authorization"), "Bearer ")
		p, err := c.Verifier.Verify(ctx.Context(), bearer)
		if !found || err != nil || scope == "" {
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "invalid operator token", errs.ErrUnauthenticated)
			return
		}
		audited := adminapi.Audited(op.Method, scope)
		entry := adminapi.Entry{Operator: p.Subject, OperationID: op.OperationID, Target: ctx.URL().Path, RequestID: &st.requestID}
		refuse := func(status int) {
			entry.Status = status
			if audited {
				st.failed = c.Store.Record(context.WithoutCancel(ctx.Context()), entry) != nil
			}
		}
		if !p.Has(scope) {
			_ = huma.WriteErr(api, ctx, http.StatusForbidden, "the operator token lacks "+scope, errs.ErrForbiddenScope)
			refuse(http.StatusForbidden)
			return
		}
		err = c.Store.Tx(ctx.Context(), p.Subject, st.requestID, func(tx adminapi.Tx) error {
			next(huma.WithContext(ctx, adminapi.WithTx(ctx.Context(), tx)))
			if st.response.status >= http.StatusBadRequest {
				return errRefused
			}
			if !audited {
				return nil
			}
			entry.Status = st.response.status
			return tx.Record(ctx.Context(), entry)
		})
		switch {
		case errors.Is(err, errRefused):
			refuse(st.response.status)
		case err != nil:
			slog.ErrorContext(ctx.Context(), "admin call not committed", "operation", op.OperationID, "err", err)
			st.failed = true
		}
	}
}
