package huma

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/martialanouman/sms-platform/internal/adminapi"
)

type webhook struct {
	ID        string    `json:"id" format:"uuid"`
	AccountID string    `json:"account_id" format:"uuid"`
	EventType string    `json:"event_type" enum:"mo,dlr"`
	URL       string    `json:"url" format:"uri"`
	Status    string    `json:"status" enum:"active,disabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type webhookWithSecret struct {
	webhook
	Secret string `json:"secret"`
}

type webhookOutput struct{ Body webhook }

type webhookSecretOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         webhookWithSecret
}

type webhookInput struct {
	ID        string `path:"id" format:"uuid"`
	WebhookID string `path:"webhook_id" format:"uuid"`
}

type createWebhookInput struct {
	ID   string `path:"id" format:"uuid"`
	Body struct {
		EventType string `json:"event_type" enum:"mo,dlr"`
		URL       string `json:"url" format:"uri" maxLength:"2048" pattern:"^https://[^/?#@]+([/?#].*)?$"`
	}
}

type updateWebhookInput struct {
	ID        string `path:"id" format:"uuid"`
	WebhookID string `path:"webhook_id" format:"uuid"`
	Body      struct {
		URL    value[string] `json:"url,omitempty" format:"uri" maxLength:"2048" pattern:"^https://[^/?#@]+([/?#].*)?$"`
		Status value[string] `json:"status,omitempty" enum:"active,disabled"`
	}
}

func revealed(w adminapi.Webhook, secret string, err error) (*webhookSecretOutput, error) {
	if err != nil {
		return nil, err
	}
	return &webhookSecretOutput{CacheControl: "no-store", Body: webhookWithSecret{webhook(w), secret}}, nil
}

func registerWebhooks(api huma.API, sealer adminapi.Sealer) {
	tags := []string{"accounts"}
	path := "/admin/accounts/{id}/webhooks"
	huma.Register(api, huma.Operation{OperationID: "list-webhooks", Method: http.MethodGet, Path: path,
		Tags: tags, Security: security("accounts:read"), Errors: []int{401, 403, 404, 422}},
		func(ctx context.Context, in *idInput) (*struct{ Body []webhook }, error) {
			webhooks, err := adminapi.AccountWebhooks(ctx, adminapi.TxFrom(ctx), in.ID)
			if err != nil {
				return nil, err
			}
			out := make([]webhook, len(webhooks))
			for i, w := range webhooks {
				out[i] = webhook(w)
			}
			return &struct{ Body []webhook }{out}, nil
		})
	huma.Register(api, huma.Operation{OperationID: "create-webhook", Method: http.MethodPost, Path: path,
		Tags: tags, Security: security("accounts:write"), DefaultStatus: http.StatusCreated, Errors: []int{401, 403, 404, 409, 422, 503}},
		func(ctx context.Context, in *createWebhookInput) (*webhookSecretOutput, error) {
			return revealed(adminapi.CreateWebhook(ctx, adminapi.TxFrom(ctx), sealer, in.ID, in.Body.EventType, in.Body.URL))
		})
	huma.Register(api, huma.Operation{OperationID: "update-webhook", Method: http.MethodPatch, Path: path + "/{webhook_id}",
		Tags: tags, Security: security("accounts:write"), Errors: []int{401, 403, 404, 409, 422}},
		func(ctx context.Context, in *updateWebhookInput) (*webhookOutput, error) {
			w, err := adminapi.UpdateWebhook(ctx, adminapi.TxFrom(ctx), in.ID, in.WebhookID,
				adminapi.WebhookPatch{URL: in.Body.URL.Value, Status: in.Body.Status.Value})
			if err != nil {
				return nil, err
			}
			return &webhookOutput{webhook(w)}, nil
		})
	huma.Register(api, huma.Operation{OperationID: "delete-webhook", Method: http.MethodDelete, Path: path + "/{webhook_id}",
		Tags: tags, Security: security("accounts:write"), DefaultStatus: http.StatusNoContent, Errors: []int{401, 403, 404, 422}},
		func(ctx context.Context, in *webhookInput) (*struct{}, error) {
			return nil, adminapi.TxFrom(ctx).DeleteWebhook(ctx, in.ID, in.WebhookID)
		})
	huma.Register(api, huma.Operation{OperationID: "rotate-webhook-secret", Method: http.MethodPost, Path: path + "/{webhook_id}/rotate-secret",
		Tags: tags, Security: security("accounts:write"), Errors: []int{401, 403, 404, 409, 422, 503}},
		func(ctx context.Context, in *webhookInput) (*webhookSecretOutput, error) {
			return revealed(adminapi.RotateWebhookSecret(ctx, adminapi.TxFrom(ctx), sealer, in.ID, in.WebhookID))
		})
}
