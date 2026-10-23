package huma

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/martialanouman/sms-platform/internal/adminapi"
)

type credential struct {
	ID             string     `json:"id" format:"uuid"`
	AccountID      string     `json:"account_id" format:"uuid"`
	Type           string     `json:"type" enum:"smpp_bind,api_key"`
	SystemID       *string    `json:"system_id"`
	Status         string     `json:"status" enum:"active,disabled,revoked"`
	LastUsedAt     *time.Time `json:"last_used_at"`
	GraceExpiresAt *time.Time `json:"grace_expires_at"`
	CreatedAt      time.Time  `json:"created_at"`
	RotatedAt      *time.Time `json:"rotated_at"`
}

type credentialWithSecret struct {
	credential
	Secret string `json:"secret"`
}

type credentialOutput struct{ Body credential }

type secretOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         credentialWithSecret
}

type credentialInput struct {
	ID           string `path:"id" format:"uuid"`
	CredentialID string `path:"credential_id" format:"uuid"`
}

type createCredentialInput struct {
	ID   string `path:"id" format:"uuid"`
	Body struct {
		Type     string  `json:"type" enum:"smpp_bind,api_key"`
		SystemID *string `json:"system_id,omitempty" minLength:"1" maxLength:"15"`
	}
}

type credentialStatusInput struct {
	ID           string `path:"id" format:"uuid"`
	CredentialID string `path:"credential_id" format:"uuid"`
	Body         struct {
		Status string `json:"status" enum:"active,disabled"`
	}
}

type rotateCredentialInput struct {
	ID           string `path:"id" format:"uuid"`
	CredentialID string `path:"credential_id" format:"uuid"`
	Body         *struct {
		GracePeriodSec nullable[int] `json:"grace_period_sec,omitempty" minimum:"0" maximum:"604800"`
	}
}

func shown(c adminapi.Credential, secret string, err error) (*secretOutput, error) {
	if err != nil {
		return nil, err
	}
	return &secretOutput{CacheControl: "no-store", Body: credentialWithSecret{credential(c), secret}}, nil
}

func registerCredentials(api huma.API) {
	tags := []string{"credentials"}
	path := "/admin/accounts/{id}/credentials"
	huma.Register(api, huma.Operation{OperationID: "list-credentials", Method: http.MethodGet, Path: path,
		Tags: tags, Security: security("credentials:read"), Errors: []int{401, 403, 404, 422}},
		func(ctx context.Context, in *idInput) (*struct{ Body []credential }, error) {
			credentials, err := adminapi.AccountCredentials(ctx, adminapi.TxFrom(ctx), in.ID)
			if err != nil {
				return nil, err
			}
			out := make([]credential, len(credentials))
			for i, c := range credentials {
				out[i] = credential(c)
			}
			return &struct{ Body []credential }{out}, nil
		})
	huma.Register(api, huma.Operation{OperationID: "create-credential", Method: http.MethodPost, Path: path,
		Tags: tags, Security: security("credentials:write"), DefaultStatus: http.StatusCreated, Errors: []int{401, 403, 404, 409, 422}},
		func(ctx context.Context, in *createCredentialInput) (*secretOutput, error) {
			return shown(adminapi.CreateCredential(ctx, adminapi.TxFrom(ctx), in.ID, in.Body.Type, in.Body.SystemID))
		})
	huma.Register(api, huma.Operation{OperationID: "update-credential-status", Method: http.MethodPatch, Path: path + "/{credential_id}",
		Tags: tags, Security: security("credentials:write"), Errors: []int{401, 403, 404, 409, 422, 503}},
		func(ctx context.Context, in *credentialStatusInput) (*credentialOutput, error) {
			c, err := adminapi.SetCredentialStatus(ctx, adminapi.TxFrom(ctx), in.ID, in.CredentialID, in.Body.Status)
			if err != nil {
				return nil, err
			}
			return &credentialOutput{credential(c)}, nil
		})
	huma.Register(api, huma.Operation{OperationID: "revoke-credential", Method: http.MethodDelete, Path: path + "/{credential_id}",
		Tags: tags, Security: security("credentials:write"), DefaultStatus: http.StatusNoContent, Errors: []int{401, 403, 404, 422, 503}},
		func(ctx context.Context, in *credentialInput) (*struct{}, error) {
			return nil, adminapi.TxFrom(ctx).RevokeCredential(ctx, in.ID, in.CredentialID)
		})
	huma.Register(api, huma.Operation{OperationID: "rotate-credential", Method: http.MethodPost, Path: path + "/{credential_id}/rotate",
		Tags: tags, Security: security("credentials:write"), Errors: []int{401, 403, 404, 409, 422, 503}},
		func(ctx context.Context, in *rotateCredentialInput) (*secretOutput, error) {
			var grace time.Duration
			if in.Body != nil && in.Body.GracePeriodSec.Value != nil {
				grace = time.Duration(*in.Body.GracePeriodSec.Value) * time.Second
			}
			return shown(adminapi.RotateCredential(ctx, adminapi.TxFrom(ctx), in.ID, in.CredentialID, grace))
		})
}
