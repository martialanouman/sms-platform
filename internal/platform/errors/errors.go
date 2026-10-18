// Package errors is the catalogue of the error codes of docs/spec/09-contrats.md §9.3 and their mapping to the REST
// and SMPP surfaces.
package errors

import "errors"

// Code is a stable error code; it is its own sentinel error.
type Code string

func (c Code) Error() string { return string(c) }

// The codes of §9.3.
const (
	ErrUnauthenticated         Code = "unauthenticated"
	ErrAccountSuspended        Code = "account_suspended"
	ErrChannelDisabled         Code = "channel_disabled"
	ErrMaxSessionsExceeded     Code = "max_sessions_exceeded"
	ErrRateLimited             Code = "rate_limited"
	ErrQuietHours              Code = "quiet_hours"
	ErrPayloadTooLarge         Code = "payload_too_large"
	ErrValidation              Code = "validation_error"
	ErrIdempotencyConflict     Code = "idempotency_conflict"
	ErrQueueFull               Code = "queue_full"
	ErrServiceUnavailable      Code = "service_unavailable"
	ErrInvalidDestination      Code = "invalid_destination"
	ErrInvalidSource           Code = "invalid_source"
	ErrSenderIDNotAuthorized   Code = "sender_id_not_authorized"
	ErrRecipientOptedOut       Code = "recipient_opted_out"
	ErrContentBlocked          Code = "content_blocked"
	ErrNoRoute                 Code = "no_route"
	ErrInsufficientCredit      Code = "insufficient_credit"
	ErrBillingUnavailable      Code = "billing_unavailable"
	ErrMessageExpired          Code = "message_expired"
	ErrSubmitFailed            Code = "submit_failed"
	ErrAmbiguousSubmit         Code = "ambiguous_submit"
	ErrIncompleteMessage       Code = "incomplete_message"
	ErrFallbackExhausted       Code = "fallback_exhausted"
	ErrRetriesExhausted        Code = "retries_exhausted"
	ErrDeliveryFailed          Code = "delivery_failed"
	ErrDeliveryExpired         Code = "delivery_expired"
	ErrMessageNotFound         Code = "message_not_found"
	ErrCancelFailed            Code = "cancel_failed"
	ErrOperationNotSupported   Code = "operation_not_supported"
	ErrNotFound                Code = "not_found"
	ErrConflict                Code = "conflict"
	ErrForbiddenScope          Code = "forbidden_scope"
	ErrForbiddenOrigin         Code = "forbidden_origin"
	ErrMFARequired             Code = "mfa_required"
	ErrUnsupportedMediaType    Code = "unsupported_media_type"
	ErrAccessLinkInvalid       Code = "access_link_invalid"
	ErrPasswordPolicy          Code = "password_policy"
	ErrMFACodeRefused          Code = "mfa_code_refused"
	ErrWebAuthnCeremonyRefused Code = "webauthn_ceremony_refused"
	ErrMFAElevationRequired    Code = "mfa_elevation_required"
	ErrMFAAlreadyEnrolled      Code = "mfa_already_enrolled"
	ErrMFAReplacementRefused   Code = "mfa_replacement_refused"
	ErrMFANothingToConfirm     Code = "mfa_nothing_to_confirm"
	ErrMFALastFactor           Code = "mfa_last_factor"
	ErrEmailTaken              Code = "email_taken"
	ErrRoleNameTaken           Code = "role_name_taken"
	ErrSelfLockout             Code = "self_lockout"
	ErrOperatorDisabled        Code = "operator_disabled"
	ErrRoleIsDefault           Code = "role_is_default"
	ErrRoleHeld                Code = "role_held"
	ErrInternal                Code = "internal_error"
)

const (
	esmeRInvMsgLen  uint32 = 0x01
	esmeRInvCmdID   uint32 = 0x03
	esmeRSysErr     uint32 = 0x08
	esmeRInvMsgID   uint32 = 0x0C
	esmeRBindFail   uint32 = 0x0D
	esmeRInvPaswd   uint32 = 0x0E
	esmeRCancelFail uint32 = 0x11
	esmeRMsgQFul    uint32 = 0x14
	esmeRSubmitFail uint32 = 0x45
	esmeRThrottled  uint32 = 0x58
)

type mapping struct {
	http int
	smpp uint32
}

var catalog = map[Code]mapping{
	ErrUnauthenticated:         {401, esmeRInvPaswd},
	ErrAccountSuspended:        {403, esmeRBindFail},
	ErrChannelDisabled:         {403, esmeRBindFail},
	ErrMaxSessionsExceeded:     {0, esmeRBindFail},
	ErrRateLimited:             {429, esmeRThrottled},
	ErrQuietHours:              {403, esmeRSubmitFail},
	ErrPayloadTooLarge:         {413, esmeRInvMsgLen},
	ErrValidation:              {422, esmeRInvMsgLen},
	ErrIdempotencyConflict:     {409, 0},
	ErrQueueFull:               {503, esmeRMsgQFul},
	ErrServiceUnavailable:      {503, esmeRSysErr},
	ErrInvalidDestination:      {},
	ErrInvalidSource:           {},
	ErrSenderIDNotAuthorized:   {},
	ErrRecipientOptedOut:       {},
	ErrContentBlocked:          {},
	ErrNoRoute:                 {},
	ErrInsufficientCredit:      {402, 0},
	ErrBillingUnavailable:      {},
	ErrMessageExpired:          {},
	ErrSubmitFailed:            {},
	ErrAmbiguousSubmit:         {},
	ErrIncompleteMessage:       {},
	ErrFallbackExhausted:       {},
	ErrRetriesExhausted:        {},
	ErrDeliveryFailed:          {},
	ErrDeliveryExpired:         {},
	ErrMessageNotFound:         {404, esmeRInvMsgID},
	ErrCancelFailed:            {409, esmeRCancelFail},
	ErrOperationNotSupported:   {405, esmeRInvCmdID},
	ErrNotFound:                {404, 0},
	ErrConflict:                {409, 0},
	ErrForbiddenScope:          {403, 0},
	ErrForbiddenOrigin:         {403, 0},
	ErrMFARequired:             {403, 0},
	ErrUnsupportedMediaType:    {415, 0},
	ErrAccessLinkInvalid:       {410, 0},
	ErrPasswordPolicy:          {422, 0},
	ErrMFACodeRefused:          {422, 0},
	ErrWebAuthnCeremonyRefused: {422, 0},
	ErrMFAElevationRequired:    {409, 0},
	ErrMFAAlreadyEnrolled:      {409, 0},
	ErrMFAReplacementRefused:   {409, 0},
	ErrMFANothingToConfirm:     {409, 0},
	ErrMFALastFactor:           {409, 0},
	ErrEmailTaken:              {409, 0},
	ErrRoleNameTaken:           {409, 0},
	ErrSelfLockout:             {409, 0},
	ErrOperatorDisabled:        {409, 0},
	ErrRoleIsDefault:           {409, 0},
	ErrRoleHeld:                {409, 0},
	ErrInternal:                {500, esmeRSysErr},
}

// HTTP returns the code and status of the REST response for err: internal_error and 500 when err carries no code
// with a REST surface.
func HTTP(err error) (Code, int) {
	var c Code
	if errors.As(err, &c) && catalog[c].http != 0 {
		return c, catalog[c].http
	}
	return ErrInternal, catalog[ErrInternal].http
}

// SMPP returns the command_status of the SMPP response for err: ESME_RSYSERR when err carries no code with an SMPP
// surface.
func SMPP(err error) uint32 {
	var c Code
	if errors.As(err, &c) && catalog[c].smpp != 0 {
		return catalog[c].smpp
	}
	return esmeRSysErr
}
