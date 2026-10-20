package http

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/martialanouman/sms-platform/internal/dashboard"
)

type server struct {
	Config
	permission map[string][]string
}

var _ StrictServerInterface = server{}

func (server) Health(context.Context, HealthRequestObject) (HealthResponseObject, error) {
	return Health200JSONResponse{Status: Ok}, nil
}

func (s server) Login(ctx context.Context, request LoginRequestObject) (LoginResponseObject, error) {
	e := from(ctx)
	login, err := s.Service.Login(ctx, request.Body.Email, request.Body.Password, s.source(e.r))
	var limited dashboard.RateLimited
	switch {
	case errors.As(err, &limited):
		retry := int(limited.RetryAfter.Seconds())
		return Login429JSONResponse{RateLimitedJSONResponse{
			Body:    Error{Code: ErrorCodeRateLimited, Message: "Trop de tentatives ; réessayez dans " + strconv.Itoa(retry) + " s."},
			Headers: RateLimitedResponseHeaders{RetryAfter: retry},
		}}, nil
	case errors.Is(err, dashboard.ErrUnauthenticated):
		return Login401JSONResponse{UnauthorizedJSONResponse{Code: ErrorCodeUnauthenticated, Message: "Adresse ou mot de passe refusé."}}, nil
	case errors.Is(err, dashboard.ErrBusy):
		return Login503JSONResponse{HashingBusyJSONResponse{Code: ErrorCodeServiceUnavailable, Message: busy}}, nil
	case err != nil:
		return nil, err
	}
	http.SetCookie(e.w, sessionCookie(login.Cookie, 0))
	return Login200JSONResponse{Challenge: login.Challenge, ExpiresAt: login.ChallengeExpiresAt}, nil
}

const busy = "Le service est saturé ; réessayez dans un instant."

func (s server) SetPasswordFromAccessLink(ctx context.Context, request SetPasswordFromAccessLinkRequestObject) (SetPasswordFromAccessLinkResponseObject, error) {
	err := s.Service.UseAccessLink(ctx, request.Body.Token, request.Body.Password)
	var policy dashboard.PolicyError
	switch {
	case errors.Is(err, dashboard.ErrLinkInvalid):
		return SetPasswordFromAccessLink410JSONResponse{Code: ErrorCodeAccessLinkInvalid, Message: "Ce lien n'est plus valable ; demandez-en un nouveau."}, nil
	case errors.As(err, &policy):
		return SetPasswordFromAccessLink422JSONResponse{Code: ErrorCodePasswordPolicy, Message: "Il manque au mot de passe : " + strings.Join(policy.Missing, ", ") + "."}, nil
	case errors.Is(err, dashboard.ErrBusy):
		return SetPasswordFromAccessLink503JSONResponse{Code: ErrorCodeServiceUnavailable, Message: busy}, nil
	case err != nil:
		return nil, err
	}
	return SetPasswordFromAccessLink204Response{}, nil
}

func (s server) Me(ctx context.Context, _ MeRequestObject) (MeResponseObject, error) {
	c, _ := ctx.Value(currentKey{}).(dashboard.Current)
	idle := int(dashboard.IdleTimeout.Seconds())
	return Me200JSONResponse{
		Operator:           CurrentOperator{Id: c.OperatorID, Email: c.Email, DisplayName: c.DisplayName},
		Permissions:        []Permission{},
		IdleTimeoutSeconds: &idle,
		AbsoluteExpiresAt:  c.ExpiresAt,
	}, nil
}

func (s server) ReportActivity(ctx context.Context, _ ReportActivityRequestObject) (ReportActivityResponseObject, error) {
	cookie, _ := from(ctx).r.Cookie(cookieName)
	err := s.Service.Activity(ctx, cookie.Value)
	switch {
	case errors.Is(err, dashboard.ErrUnauthenticated):
		return ReportActivity401JSONResponse{UnauthorizedJSONResponse{Code: ErrorCodeUnauthenticated, Message: "Aucune session ouverte."}}, nil
	case err != nil:
		return nil, err
	}
	return ReportActivity204Response{}, nil
}

func (s server) Logout(ctx context.Context, _ LogoutRequestObject) (LogoutResponseObject, error) {
	e := from(ctx)
	if cookie, err := e.r.Cookie(cookieName); err == nil {
		if err := s.Service.Logout(ctx, cookie.Value); err != nil {
			return nil, err
		}
	}
	http.SetCookie(e.w, sessionCookie("", -1))
	return Logout204Response{}, nil
}

func (s server) ListAccessLog(ctx context.Context, request ListAccessLogRequestObject) (ListAccessLogResponseObject, error) {
	var cursor string
	if request.Params.Cursor != nil {
		cursor = *request.Params.Cursor
	}
	entries, next, err := s.Service.AccessLog(ctx, cursor)
	switch {
	case errors.Is(err, dashboard.ErrBadCursor):
		return ListAccessLog422JSONResponse{ValidationErrorJSONResponse{Code: ErrorCodeValidationError, Message: "La requête est mal formée.",
			Errors: &[]fieldError{{Field: "cursor", Message: "curseur inconnu"}}}}, nil
	case err != nil:
		return nil, err
	}
	page := ListAccessLog200JSONResponse{Data: make([]AccessLogEntry, len(entries))}
	for i, e := range entries {
		outcome := AccessLogEntryOutcome("failure")
		if e.Success {
			outcome = "success"
		}
		page.Data[i] = AccessLogEntry{Id: e.ID, At: e.At, Actor: optional(e.Actor), Event: AccessLogEntryEvent(e.Event), Target: optional(e.Target), Outcome: outcome}
	}
	if next != "" {
		page.NextCursor = &next
	}
	return page, nil
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
