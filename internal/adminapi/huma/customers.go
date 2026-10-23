package huma

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/martialanouman/sms-platform/internal/adminapi"
)

type nullable[T any] adminapi.Nullable[T]

func (n *nullable[T]) UnmarshalJSON(b []byte) error {
	n.Set = true
	if string(b) == "null" {
		return nil
	}
	n.Value = new(T)
	return json.Unmarshal(b, n.Value)
}

func (nullable[T]) Schema(r huma.Registry) *huma.Schema {
	return r.Schema(reflect.TypeFor[T](), true, "")
}

type customerGroup struct {
	ID          string    `json:"id" format:"uuid"`
	Name        string    `json:"name"`
	Description *string   `json:"description" maxLength:"1000"`
	Status      string    `json:"status" enum:"active,archived"`
	MemberCount int64     `json:"member_count" minimum:"0"`
	CreatedBy   *string   `json:"created_by" format:"uuid"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type maxAge struct {
	OTP           *int `json:"otp" minimum:"1" maximum:"600"`
	Transactional *int `json:"transactional" minimum:"1" maximum:"86400"`
	Marketing     *int `json:"marketing" minimum:"1" maximum:"259200"`
}

type customer struct {
	ID             string    `json:"id" format:"uuid"`
	Name           string    `json:"name"`
	Status         string    `json:"status" enum:"active,suspended,closed"`
	GroupID        *string   `json:"group_id" format:"uuid"`
	MaxAgeOverride maxAge    `json:"max_age_override"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type groupOutput struct{ Body customerGroup }

type idInput struct {
	ID string `path:"id" format:"uuid"`
}

type listGroupsInput struct {
	Status string `query:"status" enum:"active,archived"`
	Cursor string `query:"cursor" maxLength:"1024"`
	Limit  int    `query:"limit" minimum:"1" maximum:"500" default:"50"`
}

type listGroupCustomersInput struct {
	ID     string `path:"id" format:"uuid"`
	Cursor string `query:"cursor" maxLength:"1024"`
	Limit  int    `query:"limit" minimum:"1" maximum:"500" default:"50"`
}

type createGroupInput struct {
	Body struct {
		Name        string  `json:"name" minLength:"1" maxLength:"200"`
		Description *string `json:"description,omitempty" maxLength:"1000"`
	}
}

type updateGroupInput struct {
	ID   string `path:"id" format:"uuid"`
	Body struct {
		Name        *string          `json:"name,omitempty" minLength:"1" maxLength:"200"`
		Description nullable[string] `json:"description,omitempty" maxLength:"1000"`
		Status      *string          `json:"status,omitempty" enum:"active,archived"`
	}
}

func toGroup(g adminapi.CustomerGroup) customerGroup {
	return customerGroup{ID: g.ID, Name: g.Name, Description: g.Description, Status: g.Status, MemberCount: g.MemberCount,
		CreatedBy: g.CreatedBy, CreatedAt: g.CreatedAt, UpdatedAt: g.UpdatedAt}
}

func toCustomer(c adminapi.Customer) customer {
	return customer{ID: c.ID, Name: c.Name, Status: c.Status, GroupID: c.GroupID, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
		MaxAgeOverride: maxAge{OTP: c.MaxAge.OTP, Transactional: c.MaxAge.Transactional, Marketing: c.MaxAge.Marketing}}
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func registerCustomerGroups(api huma.API) {
	tags := []string{"customer-groups"}
	huma.Register(api, huma.Operation{OperationID: "list-customer-groups", Method: http.MethodGet, Path: "/admin/customer-groups",
		Tags: tags, Security: security("customer-groups:read"), Errors: []int{401, 403, 422}},
		func(ctx context.Context, in *listGroupsInput) (*pageOutput[customerGroup], error) {
			groups, next, err := adminapi.GroupsPage(ctx, adminapi.TxFrom(ctx), optional(in.Status), in.Cursor, in.Limit)
			if err != nil {
				return nil, err
			}
			out := make([]customerGroup, len(groups))
			for i, g := range groups {
				out[i] = toGroup(g)
			}
			return page(out, next), nil
		})
	huma.Register(api, huma.Operation{OperationID: "create-customer-group", Method: http.MethodPost, Path: "/admin/customer-groups",
		Tags: tags, Security: security("customer-groups:write"), DefaultStatus: http.StatusCreated, Errors: []int{401, 403, 409, 422}},
		func(ctx context.Context, in *createGroupInput) (*groupOutput, error) {
			g, err := adminapi.TxFrom(ctx).CreateGroup(ctx, in.Body.Name, in.Body.Description)
			if err != nil {
				return nil, err
			}
			return &groupOutput{toGroup(g)}, nil
		})
	huma.Register(api, huma.Operation{OperationID: "get-customer-group", Method: http.MethodGet, Path: "/admin/customer-groups/{id}",
		Tags: tags, Security: security("customer-groups:read"), Errors: []int{401, 403, 404, 422}},
		func(ctx context.Context, in *idInput) (*groupOutput, error) {
			g, err := adminapi.TxFrom(ctx).Group(ctx, in.ID)
			if err != nil {
				return nil, err
			}
			return &groupOutput{toGroup(g)}, nil
		})
	huma.Register(api, huma.Operation{OperationID: "update-customer-group", Method: http.MethodPatch, Path: "/admin/customer-groups/{id}",
		Tags: tags, Security: security("customer-groups:write"), Errors: []int{401, 403, 404, 409, 422}},
		func(ctx context.Context, in *updateGroupInput) (*groupOutput, error) {
			g, err := adminapi.TxFrom(ctx).UpdateGroup(ctx, in.ID, adminapi.GroupPatch{Name: in.Body.Name,
				Description: adminapi.Nullable[string](in.Body.Description), Status: in.Body.Status})
			if err != nil {
				return nil, err
			}
			return &groupOutput{toGroup(g)}, nil
		})
	huma.Register(api, huma.Operation{OperationID: "delete-customer-group", Method: http.MethodDelete, Path: "/admin/customer-groups/{id}",
		Tags: tags, Security: security("customer-groups:write"), DefaultStatus: http.StatusNoContent, Errors: []int{401, 403, 404, 422}},
		func(ctx context.Context, in *idInput) (*struct{}, error) {
			return nil, adminapi.TxFrom(ctx).DeleteGroup(ctx, in.ID)
		})
	huma.Register(api, huma.Operation{OperationID: "list-group-customers", Method: http.MethodGet, Path: "/admin/customer-groups/{id}/customers",
		Tags: tags, Security: security("customer-groups:read"), Errors: []int{401, 403, 404, 422}},
		func(ctx context.Context, in *listGroupCustomersInput) (*pageOutput[customer], error) {
			customers, next, err := adminapi.GroupCustomersPage(ctx, adminapi.TxFrom(ctx), in.ID, in.Cursor, in.Limit)
			if err != nil {
				return nil, err
			}
			out := make([]customer, len(customers))
			for i, c := range customers {
				out[i] = toCustomer(c)
			}
			return page(out, next), nil
		})
}
