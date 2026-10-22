package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/martialanouman/sms-platform/internal/dashboard"
)

type server struct {
	Config
	permission map[string][]string
	auth       map[string]bool
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
		return Login429JSONResponse{tooMany(limited)}, nil
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

func tooMany(limited dashboard.RateLimited) RateLimitedJSONResponse {
	retry := int(limited.RetryAfter.Seconds())
	return RateLimitedJSONResponse{
		Body:    Error{Code: ErrorCodeRateLimited, Message: "Trop de tentatives ; réessayez dans " + strconv.Itoa(retry) + " s."},
		Headers: RateLimitedResponseHeaders{RetryAfter: retry},
	}
}

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
		Permissions:        c.Permissions,
		Elevated:           c.Elevated,
		SecondFactors:      SecondFactors{Totp: c.Totp, RecoveryCodesRemaining: c.RecoveryCodesRemaining, Passkeys: c.Passkeys},
		IdleTimeoutSeconds: &idle,
		AbsoluteExpiresAt:  c.ExpiresAt,
	}, nil
}

func (s server) ReportActivity(ctx context.Context, _ ReportActivityRequestObject) (ReportActivityResponseObject, error) {
	cookie, _ := from(ctx).r.Cookie(cookieName)
	err := s.Service.Activity(ctx, cookie.Value)
	switch {
	case errors.Is(err, dashboard.ErrUnauthenticated):
		return ReportActivity401JSONResponse{UnauthorizedJSONResponse{Code: ErrorCodeUnauthenticated, Message: noSession}}, nil
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

const elevationRequired = "Cette opération demande une vérification récente du second facteur : reconnectez-vous."

const factorRefused = "Second facteur refusé ; si la connexion date de plus de 5 minutes, reconnectez-vous."

func (s server) EnrollTotp(ctx context.Context, request EnrollTotpRequestObject) (EnrollTotpResponseObject, error) {
	var proof *dashboard.Proof
	if request.Body.Method != nil {
		proof = &dashboard.Proof{Method: dashboard.Method(*request.Body.Method), Code: *request.Body.Code}
	}
	e, err := s.Service.EnrollTotp(ctx, cookie(ctx), proof)
	var limited dashboard.RateLimited
	switch {
	case errors.As(err, &limited):
		return EnrollTotp429JSONResponse{tooMany(limited)}, nil
	case errors.Is(err, dashboard.ErrElevationRequired):
		return EnrollTotp409JSONResponse{Code: ErrorCodeMfaElevationRequired, Message: elevationRequired}, nil
	case errors.Is(err, dashboard.ErrAlreadyEnrolled):
		return EnrollTotp409JSONResponse{Code: ErrorCodeMfaAlreadyEnrolled, Message: "Un facteur est en place ; présentez-en un code pour le remplacer."}, nil
	case errors.Is(err, dashboard.ErrReplacementRefused):
		return EnrollTotp409JSONResponse{Code: ErrorCodeMfaReplacementRefused, Message: "Le code du facteur en place est refusé."}, nil
	case errors.Is(err, dashboard.ErrUnauthenticated):
		return EnrollTotp401JSONResponse{UnauthorizedJSONResponse{Code: ErrorCodeUnauthenticated, Message: noSession}}, nil
	case err != nil:
		return nil, err
	}
	return EnrollTotp200JSONResponse{Secret: e.Secret, OtpauthUri: e.URI, RecoveryCodes: e.RecoveryCodes}, nil
}

func (s server) ConfirmTotp(ctx context.Context, request ConfirmTotpRequestObject) (ConfirmTotpResponseObject, error) {
	err := s.Service.ConfirmTotp(ctx, cookie(ctx), request.Body.Code)
	var limited dashboard.RateLimited
	switch {
	case errors.As(err, &limited):
		return ConfirmTotp429JSONResponse{tooMany(limited)}, nil
	case errors.Is(err, dashboard.ErrElevationRequired):
		return ConfirmTotp409JSONResponse{Code: ErrorCodeMfaElevationRequired, Message: elevationRequired}, nil
	case errors.Is(err, dashboard.ErrNothingToConfirm):
		return ConfirmTotp409JSONResponse{Code: ErrorCodeMfaNothingToConfirm, Message: "Aucune application n'attend de confirmation."}, nil
	case errors.Is(err, dashboard.ErrCodeRefused):
		return ConfirmTotp422JSONResponse{Code: ErrorCodeMfaCodeRefused, Message: "Ce code n'est pas accepté."}, nil
	case errors.Is(err, dashboard.ErrUnauthenticated):
		return ConfirmTotp401JSONResponse{UnauthorizedJSONResponse{Code: ErrorCodeUnauthenticated, Message: noSession}}, nil
	case err != nil:
		return nil, err
	}
	return ConfirmTotp204Response{}, nil
}

func (s server) VerifyMfa(ctx context.Context, request VerifyMfaRequestObject) (VerifyMfaResponseObject, error) {
	var code string
	if request.Body.Code != nil {
		code = *request.Body.Code
	}
	proof := dashboard.Proof{Method: dashboard.Method(request.Body.Method), Code: code}
	if request.Body.Assertion != nil {
		proof.Assertion, _ = json.Marshal(*request.Body.Assertion)
	}
	next, err := s.Service.Verify(ctx, cookie(ctx), request.Body.Challenge, proof)
	var limited dashboard.RateLimited
	switch {
	case errors.As(err, &limited):
		return VerifyMfa429JSONResponse{tooMany(limited)}, nil
	case errors.Is(err, dashboard.ErrUnauthenticated):
		return VerifyMfa401JSONResponse{Code: ErrorCodeUnauthenticated, Message: factorRefused}, nil
	case err != nil:
		return nil, err
	}
	http.SetCookie(from(ctx).w, sessionCookie(next, 0))
	return VerifyMfa204Response{}, nil
}

func cookie(ctx context.Context) string {
	c, _ := from(ctx).r.Cookie(cookieName)
	return c.Value
}

func (s server) ListAccessLog(ctx context.Context, request ListAccessLogRequestObject) (ListAccessLogResponseObject, error) {
	var cursor string
	if request.Params.Cursor != nil {
		cursor = *request.Params.Cursor
	}
	f := dashboard.AccessLogFilter{From: request.Params.FromDate, To: request.Params.ToDate}
	if request.Params.Actor != nil {
		f.Actor = new(request.Params.Actor.String())
	}
	entries, next, err := s.Service.AccessLog(ctx, f, cursor)
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

const noSession = "Aucune session ouverte."

func (s server) BeginWebauthnRegistration(ctx context.Context, _ BeginWebauthnRegistrationRequestObject) (BeginWebauthnRegistrationResponseObject, error) {
	options, err := s.Service.BeginRegistration(ctx, cookie(ctx))
	var out BeginWebauthnRegistration200JSONResponse
	switch {
	case errors.Is(err, dashboard.ErrElevationRequired):
		return BeginWebauthnRegistration409JSONResponse{Code: ErrorCodeMfaElevationRequired, Message: elevationRequired}, nil
	case errors.Is(err, dashboard.ErrUnauthenticated):
		return BeginWebauthnRegistration401JSONResponse{UnauthorizedJSONResponse{Code: ErrorCodeUnauthenticated, Message: noSession}}, nil
	case err != nil:
		return nil, err
	}
	return out, json.Unmarshal(options, &out)
}

func (s server) FinishWebauthnRegistration(ctx context.Context, request FinishWebauthnRegistrationRequestObject) (FinishWebauthnRegistrationResponseObject, error) {
	attestation, err := json.Marshal(request.Body.Attestation)
	if err != nil {
		return nil, err
	}
	id, err := s.Service.FinishRegistration(ctx, cookie(ctx), request.Body.Name, attestation)
	switch {
	case errors.Is(err, dashboard.ErrElevationRequired):
		return FinishWebauthnRegistration409JSONResponse{Code: ErrorCodeMfaElevationRequired, Message: elevationRequired}, nil
	case errors.Is(err, dashboard.ErrCeremonyRefused):
		return FinishWebauthnRegistration422JSONResponse{Code: ErrorCodeWebauthnCeremonyRefused, Message: "La clé d'accès n'a pas été enregistrée ; recommencez."}, nil
	case errors.Is(err, dashboard.ErrUnauthenticated):
		return FinishWebauthnRegistration401JSONResponse{UnauthorizedJSONResponse{Code: ErrorCodeUnauthenticated, Message: noSession}}, nil
	case err != nil:
		return nil, err
	}
	return FinishWebauthnRegistration200JSONResponse{Id: id}, nil
}

func (s server) BeginWebauthnAssertion(ctx context.Context, _ BeginWebauthnAssertionRequestObject) (BeginWebauthnAssertionResponseObject, error) {
	options, err := s.Service.BeginAssertion(ctx, cookie(ctx))
	var out BeginWebauthnAssertion200JSONResponse
	switch {
	case errors.Is(err, dashboard.ErrNoPasskey):
		return BeginWebauthnAssertion404JSONResponse{Code: ErrorCodeNotFound, Message: "Aucune clé d'accès n'est enregistrée."}, nil
	case errors.Is(err, dashboard.ErrUnauthenticated):
		return BeginWebauthnAssertion401JSONResponse{UnauthorizedJSONResponse{Code: ErrorCodeUnauthenticated, Message: noSession}}, nil
	case err != nil:
		return nil, err
	}
	return out, json.Unmarshal(options, &out)
}

func (s server) ListWebauthnPasskeys(ctx context.Context, _ ListWebauthnPasskeysRequestObject) (ListWebauthnPasskeysResponseObject, error) {
	keys, err := s.Service.ListPasskeys(ctx, cookie(ctx))
	switch {
	case errors.Is(err, dashboard.ErrUnauthenticated):
		return ListWebauthnPasskeys401JSONResponse{UnauthorizedJSONResponse{Code: ErrorCodeUnauthenticated, Message: noSession}}, nil
	case err != nil:
		return nil, err
	}
	out := make(ListWebauthnPasskeys200JSONResponse, len(keys))
	for i, k := range keys {
		out[i] = PasskeySummary{Id: k.ID, Name: k.Name, CreatedAt: k.CreatedAt}
	}
	return out, nil
}

func (s server) DeleteWebauthnPasskey(ctx context.Context, request DeleteWebauthnPasskeyRequestObject) (DeleteWebauthnPasskeyResponseObject, error) {
	err := s.Service.RemovePasskey(ctx, cookie(ctx), request.PasskeyId)
	switch {
	case errors.Is(err, dashboard.ErrElevationRequired):
		return DeleteWebauthnPasskey409JSONResponse{Code: ErrorCodeMfaElevationRequired, Message: elevationRequired}, nil
	case errors.Is(err, dashboard.ErrLastFactor):
		return DeleteWebauthnPasskey409JSONResponse{Code: ErrorCodeMfaLastFactor, Message: "C'est votre dernier facteur : ajoutez-en un autre avant de le retirer."}, nil
	case errors.Is(err, dashboard.ErrPasskeyUnknown):
		return DeleteWebauthnPasskey404JSONResponse{Code: ErrorCodeNotFound, Message: "Cette clé d'accès est inconnue."}, nil
	case errors.Is(err, dashboard.ErrUnauthenticated):
		return DeleteWebauthnPasskey401JSONResponse{UnauthorizedJSONResponse{Code: ErrorCodeUnauthenticated, Message: noSession}}, nil
	case err != nil:
		return nil, err
	}
	return DeleteWebauthnPasskey204Response{}, nil
}
