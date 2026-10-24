package huma

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/martialanouman/sms-platform/internal/adminapi"
)

type account struct {
	ID               string    `json:"id" format:"uuid"`
	CustomerID       string    `json:"customer_id" format:"uuid"`
	CustomerName     string    `json:"customer_name"`
	CustomerStatus   string    `json:"customer_status" enum:"active,suspended,closed"`
	Name             string    `json:"name"`
	Status           string    `json:"status" enum:"active,suspended,closed"`
	SmppEnabled      bool      `json:"smpp_enabled"`
	RestEnabled      bool      `json:"rest_enabled"`
	QuerySmEnabled   bool      `json:"query_sm_enabled"`
	CancelSmEnabled  bool      `json:"cancel_sm_enabled"`
	AllowedBindTypes []string  `json:"allowed_bind_types" uniqueItems:"true" enum:"transmitter,receiver,transceiver"`
	MaxSessions      int       `json:"max_sessions" minimum:"0"`
	RateLimit        *struct{} `json:"rate_limit"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type accountOutput struct{ Body account }

type listAccountsInput struct {
	CustomerID string `query:"customer_id" format:"uuid"`
	GroupID    string `query:"group_id" format:"uuid"`
	Status     string `query:"status" enum:"active,suspended,closed"`
	Cursor     string `query:"cursor" maxLength:"1024"`
	Limit      int    `query:"limit" minimum:"1" maximum:"500" default:"50"`
}

type createAccountInput struct {
	Body struct {
		CustomerID       string          `json:"customer_id" format:"uuid"`
		Name             string          `json:"name" minLength:"1" maxLength:"200"`
		SmppEnabled      *bool           `json:"smpp_enabled,omitempty" default:"true"`
		RestEnabled      *bool           `json:"rest_enabled,omitempty" default:"true"`
		QuerySmEnabled   *bool           `json:"query_sm_enabled,omitempty" default:"true"`
		CancelSmEnabled  *bool           `json:"cancel_sm_enabled,omitempty" default:"true"`
		AllowedBindTypes value[[]string] `json:"allowed_bind_types,omitempty" nullable:"false" minItems:"1" uniqueItems:"true" enum:"transmitter,receiver,transceiver"`
		MaxSessions      *int            `json:"max_sessions,omitempty" minimum:"0" maximum:"2147483647" default:"1"`
	}
}

type updateAccountInput struct {
	ID   string `path:"id" format:"uuid"`
	Body struct {
		Name   value[string] `json:"name,omitempty" minLength:"1" maxLength:"200"`
		Status value[string] `json:"status,omitempty" enum:"active,suspended,closed"`
	}
}

type channelsInput struct {
	ID   string `path:"id" format:"uuid"`
	Body struct {
		SmppEnabled bool `json:"smpp_enabled"`
		RestEnabled bool `json:"rest_enabled"`
	}
}

type sessionLimitsInput struct {
	ID   string `path:"id" format:"uuid"`
	Body struct {
		MaxSessions      int      `json:"max_sessions" minimum:"0" maximum:"2147483647"`
		AllowedBindTypes []string `json:"allowed_bind_types" nullable:"false" minItems:"1" uniqueItems:"true" enum:"transmitter,receiver,transceiver"`
	}
}

type smppOpsInput struct {
	ID   string `path:"id" format:"uuid"`
	Body struct {
		QuerySmEnabled  value[bool] `json:"query_sm_enabled,omitempty"`
		CancelSmEnabled value[bool] `json:"cancel_sm_enabled,omitempty"`
	}
}

func registerAccounts(api huma.API) {
	tags := []string{"accounts"}
	out := func(a adminapi.Account, err error) (*accountOutput, error) {
		if err != nil {
			return nil, err
		}
		return &accountOutput{toAccount(a)}, nil
	}
	huma.Register(api, huma.Operation{OperationID: "list-accounts", Method: http.MethodGet, Path: "/admin/accounts",
		Tags: tags, Security: security("accounts:read"), Errors: []int{401, 403, 422}},
		func(ctx context.Context, in *listAccountsInput) (*countedPageOutput[account], error) {
			var f adminapi.AccountFilter
			if in.CustomerID != "" {
				f.CustomerID = &in.CustomerID
			}
			if in.GroupID != "" {
				f.GroupID = &in.GroupID
			}
			if in.Status != "" {
				f.Status = &in.Status
			}
			accounts, next, total, err := adminapi.AccountsPage(ctx, adminapi.TxFrom(ctx), f, in.Cursor, in.Limit)
			if err != nil {
				return nil, err
			}
			data := make([]account, len(accounts))
			for i, a := range accounts {
				data[i] = toAccount(a)
			}
			return counted(data, next, total), nil
		})
	huma.Register(api, huma.Operation{OperationID: "create-account", Method: http.MethodPost, Path: "/admin/accounts",
		Tags: tags, Security: security("accounts:write"), DefaultStatus: http.StatusCreated, Errors: []int{401, 403, 409, 422}},
		func(ctx context.Context, in *createAccountInput) (*accountOutput, error) {
			b := in.Body
			types := []string{"transceiver"}
			if b.AllowedBindTypes.Value != nil {
				types = *b.AllowedBindTypes.Value
			}
			return out(adminapi.TxFrom(ctx).CreateAccount(ctx, adminapi.Account{CustomerID: b.CustomerID, Name: b.Name,
				SmppEnabled: *b.SmppEnabled, RestEnabled: *b.RestEnabled, QuerySmEnabled: *b.QuerySmEnabled,
				CancelSmEnabled: *b.CancelSmEnabled, AllowedBindTypes: types, MaxSessions: *b.MaxSessions}))
		})
	huma.Register(api, huma.Operation{OperationID: "get-account", Method: http.MethodGet, Path: "/admin/accounts/{id}",
		Tags: tags, Security: security("accounts:read"), Errors: []int{401, 403, 404, 422}},
		func(ctx context.Context, in *idInput) (*accountOutput, error) {
			return out(adminapi.TxFrom(ctx).Account(ctx, in.ID))
		})
	huma.Register(api, huma.Operation{OperationID: "update-account", Method: http.MethodPatch, Path: "/admin/accounts/{id}",
		Tags: tags, Security: security("accounts:write"), Errors: []int{401, 403, 404, 409, 422, 503}},
		func(ctx context.Context, in *updateAccountInput) (*accountOutput, error) {
			return out(adminapi.TxFrom(ctx).UpdateAccount(ctx, in.ID,
				adminapi.AccountPatch{Name: in.Body.Name.Value, Status: in.Body.Status.Value}))
		})
	huma.Register(api, huma.Operation{OperationID: "delete-account", Method: http.MethodDelete, Path: "/admin/accounts/{id}",
		Tags: tags, Security: security("accounts:write"), DefaultStatus: http.StatusNoContent, Errors: []int{401, 403, 404, 409, 422}},
		func(ctx context.Context, in *idInput) (*struct{}, error) {
			return nil, adminapi.TxFrom(ctx).DeleteAccount(ctx, in.ID)
		})
	huma.Register(api, huma.Operation{OperationID: "suspend-account", Method: http.MethodPost, Path: "/admin/accounts/{id}/suspend",
		Tags: tags, Security: security("accounts:write"), Errors: []int{401, 403, 404, 409, 422, 503}},
		func(ctx context.Context, in *idInput) (*accountOutput, error) {
			return out(adminapi.SuspendAccount(ctx, adminapi.TxFrom(ctx), in.ID))
		})
	huma.Register(api, huma.Operation{OperationID: "set-account-channels", Method: http.MethodPatch, Path: "/admin/accounts/{id}/channels",
		Tags: tags, Security: security("accounts:write"), Errors: []int{401, 403, 404, 422}},
		func(ctx context.Context, in *channelsInput) (*accountOutput, error) {
			return out(adminapi.TxFrom(ctx).UpdateAccount(ctx, in.ID,
				adminapi.AccountPatch{SmppEnabled: &in.Body.SmppEnabled, RestEnabled: &in.Body.RestEnabled}))
		})
	huma.Register(api, huma.Operation{OperationID: "set-account-session-limits", Method: http.MethodPatch,
		Path: "/admin/accounts/{id}/session-limits", Tags: tags, Security: security("accounts:write"), Errors: []int{401, 403, 404, 422}},
		func(ctx context.Context, in *sessionLimitsInput) (*accountOutput, error) {
			return out(adminapi.TxFrom(ctx).UpdateAccount(ctx, in.ID,
				adminapi.AccountPatch{MaxSessions: &in.Body.MaxSessions, AllowedBindTypes: in.Body.AllowedBindTypes}))
		})
	huma.Register(api, huma.Operation{OperationID: "set-account-smpp-ops", Method: http.MethodPatch, Path: "/admin/accounts/{id}/smpp-ops",
		Tags: tags, Security: security("accounts:write"), Errors: []int{401, 403, 404, 422, 503}},
		func(ctx context.Context, in *smppOpsInput) (*accountOutput, error) {
			q, c := in.Body.QuerySmEnabled.Value, in.Body.CancelSmEnabled.Value
			if q == nil && c == nil {
				return nil, huma.Error422UnprocessableEntity("at least one flag is set")
			}
			return out(adminapi.TxFrom(ctx).UpdateAccount(ctx, in.ID, adminapi.AccountPatch{QuerySmEnabled: q, CancelSmEnabled: c}))
		})
}

func toAccount(a adminapi.Account) account {
	return account{ID: a.ID, CustomerID: a.CustomerID, CustomerName: a.CustomerName, CustomerStatus: a.CustomerStatus,
		Name: a.Name, Status: a.Status, SmppEnabled: a.SmppEnabled, RestEnabled: a.RestEnabled,
		QuerySmEnabled: a.QuerySmEnabled, CancelSmEnabled: a.CancelSmEnabled, AllowedBindTypes: a.AllowedBindTypes,
		MaxSessions: a.MaxSessions, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt}
}
