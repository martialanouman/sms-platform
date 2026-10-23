package huma

import (
	"context"
	"encoding/json"
	"errors"
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

type value[T any] struct{ Value *T }

var errNull = errors.New("must not be null")

func (v *value[T]) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		return errNull
	}
	v.Value = new(T)
	return json.Unmarshal(b, v.Value)
}

func (value[T]) Schema(r huma.Registry) *huma.Schema {
	return r.Schema(reflect.TypeFor[T](), true, "")
}

type customerGroup struct {
	ID          string    `json:"id" format:"uuid"`
	Name        string    `json:"name"`
	Description *string   `json:"description" maxLength:"1000"`
	Status      string    `json:"status" enum:"active,archived"`
	CreatedBy   *string   `json:"created_by" format:"uuid"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	MemberCount int64     `json:"member_count" minimum:"0"`
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
		Name        value[string]    `json:"name,omitempty" minLength:"1" maxLength:"200"`
		Description nullable[string] `json:"description,omitempty" maxLength:"1000"`
		Status      value[string]    `json:"status,omitempty" enum:"active,archived"`
	}
}

func toCustomer(c adminapi.Customer) customer {
	return customer{ID: c.ID, Name: c.Name, Status: c.Status, GroupID: c.GroupID, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
		MaxAgeOverride: maxAge(c.MaxAge)}
}

func toCustomers(customers []adminapi.Customer) []customer {
	out := make([]customer, len(customers))
	for i, c := range customers {
		out[i] = toCustomer(c)
	}
	return out
}

func registerCustomerGroups(api huma.API) {
	tags := []string{"customer-groups"}
	huma.Register(api, huma.Operation{OperationID: "list-customer-groups", Method: http.MethodGet, Path: "/admin/customer-groups",
		Tags: tags, Security: security("customer-groups:read"), Errors: []int{401, 403, 422}},
		func(ctx context.Context, in *listGroupsInput) (*pageOutput[customerGroup], error) {
			var status *string
			if in.Status != "" {
				status = &in.Status
			}
			groups, next, err := adminapi.GroupsPage(ctx, adminapi.TxFrom(ctx), status, in.Cursor, in.Limit)
			if err != nil {
				return nil, err
			}
			out := make([]customerGroup, len(groups))
			for i, g := range groups {
				out[i] = customerGroup(g)
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
			return &groupOutput{customerGroup(g)}, nil
		})
	huma.Register(api, huma.Operation{OperationID: "get-customer-group", Method: http.MethodGet, Path: "/admin/customer-groups/{id}",
		Tags: tags, Security: security("customer-groups:read"), Errors: []int{401, 403, 404, 422}},
		func(ctx context.Context, in *idInput) (*groupOutput, error) {
			g, err := adminapi.TxFrom(ctx).Group(ctx, in.ID)
			if err != nil {
				return nil, err
			}
			return &groupOutput{customerGroup(g)}, nil
		})
	huma.Register(api, huma.Operation{OperationID: "update-customer-group", Method: http.MethodPatch, Path: "/admin/customer-groups/{id}",
		Tags: tags, Security: security("customer-groups:write"), Errors: []int{401, 403, 404, 409, 422}},
		func(ctx context.Context, in *updateGroupInput) (*groupOutput, error) {
			g, err := adminapi.TxFrom(ctx).UpdateGroup(ctx, in.ID, adminapi.GroupPatch{Name: in.Body.Name.Value,
				Description: adminapi.Nullable[string](in.Body.Description), Status: in.Body.Status.Value})
			if err != nil {
				return nil, err
			}
			return &groupOutput{customerGroup(g)}, nil
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
			return page(toCustomers(customers), next), nil
		})
}

type customerOutput struct{ Body customer }

type listCustomersInput struct {
	GroupID string `query:"group_id" format:"uuid"`
	Status  string `query:"status" enum:"active,suspended,closed"`
	Cursor  string `query:"cursor" maxLength:"1024"`
	Limit   int    `query:"limit" minimum:"1" maximum:"500" default:"50"`
}

type createCustomerInput struct {
	Body struct {
		Name           string        `json:"name" minLength:"1" maxLength:"200"`
		GroupID        *string       `json:"group_id,omitempty" format:"uuid"`
		MaxAgeOverride value[maxAge] `json:"max_age_override,omitempty"`
	}
}

type updateCustomerInput struct {
	ID   string `path:"id" format:"uuid"`
	Body struct {
		Name           value[string]    `json:"name,omitempty" minLength:"1" maxLength:"200"`
		Status         value[string]    `json:"status,omitempty" enum:"active,suspended,closed"`
		GroupID        nullable[string] `json:"group_id,omitempty" format:"uuid"`
		MaxAgeOverride value[maxAge]    `json:"max_age_override,omitempty"`
	}
}

type accountRef struct {
	ID   string `json:"id" format:"uuid"`
	Name string `json:"name"`
}

type suspensionImpactOutput struct {
	Body struct {
		AccountsToSuspend []accountRef `json:"accounts_to_suspend"`
		AccountsClosed    []accountRef `json:"accounts_closed"`
		BindsToDisconnect []any        `json:"binds_to_disconnect"`
	}
}

func refs(accounts []adminapi.AccountRef) []accountRef {
	out := make([]accountRef, len(accounts))
	for i, a := range accounts {
		out[i] = accountRef{ID: a.ID, Name: a.Name}
	}
	return out
}

func registerCustomers(api huma.API) {
	tags := []string{"customers"}
	huma.Register(api, huma.Operation{OperationID: "list-customers", Method: http.MethodGet, Path: "/admin/customers",
		Tags: tags, Security: security("customers:read"), Errors: []int{401, 403, 422}},
		func(ctx context.Context, in *listCustomersInput) (*pageOutput[customer], error) {
			var f adminapi.CustomerFilter
			if in.GroupID != "" {
				f.GroupID = &in.GroupID
			}
			if in.Status != "" {
				f.Status = &in.Status
			}
			customers, next, err := adminapi.CustomersPage(ctx, adminapi.TxFrom(ctx), f, in.Cursor, in.Limit)
			if err != nil {
				return nil, err
			}
			return page(toCustomers(customers), next), nil
		})
	huma.Register(api, huma.Operation{OperationID: "create-customer", Method: http.MethodPost, Path: "/admin/customers",
		Tags: tags, Security: security("customers:write"), DefaultStatus: http.StatusCreated, Errors: []int{401, 403, 422}},
		func(ctx context.Context, in *createCustomerInput) (*customerOutput, error) {
			var age adminapi.MaxAge
			if v := in.Body.MaxAgeOverride.Value; v != nil {
				age = adminapi.MaxAge(*v)
			}
			c, err := adminapi.TxFrom(ctx).CreateCustomer(ctx, in.Body.Name, in.Body.GroupID, age)
			if err != nil {
				return nil, err
			}
			return &customerOutput{toCustomer(c)}, nil
		})
	huma.Register(api, huma.Operation{OperationID: "get-customer", Method: http.MethodGet, Path: "/admin/customers/{id}",
		Tags: tags, Security: security("customers:read"), Errors: []int{401, 403, 404, 422}},
		func(ctx context.Context, in *idInput) (*customerOutput, error) {
			c, err := adminapi.TxFrom(ctx).Customer(ctx, in.ID)
			if err != nil {
				return nil, err
			}
			return &customerOutput{toCustomer(c)}, nil
		})
	huma.Register(api, huma.Operation{OperationID: "update-customer", Method: http.MethodPatch, Path: "/admin/customers/{id}",
		Tags: tags, Security: security("customers:write"), Errors: []int{401, 403, 404, 422}},
		func(ctx context.Context, in *updateCustomerInput) (*customerOutput, error) {
			p := adminapi.CustomerPatch{Name: in.Body.Name.Value, Status: in.Body.Status.Value,
				GroupID: adminapi.Nullable[string](in.Body.GroupID)}
			if v := in.Body.MaxAgeOverride.Value; v != nil {
				age := adminapi.MaxAge(*v)
				p.MaxAge = &age
			}
			c, err := adminapi.UpdateCustomer(ctx, adminapi.TxFrom(ctx), in.ID, p)
			if err != nil {
				return nil, err
			}
			return &customerOutput{toCustomer(c)}, nil
		})
	huma.Register(api, huma.Operation{OperationID: "delete-customer", Method: http.MethodDelete, Path: "/admin/customers/{id}",
		Tags: tags, Security: security("customers:write"), DefaultStatus: http.StatusNoContent, Errors: []int{401, 403, 404, 409, 422}},
		func(ctx context.Context, in *idInput) (*struct{}, error) {
			return nil, adminapi.TxFrom(ctx).DeleteCustomer(ctx, in.ID)
		})
	huma.Register(api, huma.Operation{OperationID: "suspend-customer", Method: http.MethodPost, Path: "/admin/customers/{id}/suspend",
		Tags: tags, Security: security("customers:write"), Errors: []int{401, 403, 404, 409, 422, 503}},
		func(ctx context.Context, in *idInput) (*customerOutput, error) {
			c, err := adminapi.SuspendCustomer(ctx, adminapi.TxFrom(ctx), in.ID)
			if err != nil {
				return nil, err
			}
			return &customerOutput{toCustomer(c)}, nil
		})
	huma.Register(api, huma.Operation{OperationID: "get-customer-suspension-impact", Method: http.MethodGet,
		Path: "/admin/customers/{id}/suspension-impact", Tags: tags, Security: security("customers:read"),
		Errors: []int{401, 403, 404, 422, 503}},
		func(ctx context.Context, in *idInput) (*suspensionImpactOutput, error) {
			suspend, closed, err := adminapi.SuspensionImpact(ctx, adminapi.TxFrom(ctx), in.ID)
			if err != nil {
				return nil, err
			}
			out := &suspensionImpactOutput{}
			out.Body.AccountsToSuspend, out.Body.AccountsClosed = refs(suspend), refs(closed)
			// debt: #173
			out.Body.BindsToDisconnect = []any{}
			return out, nil
		})
}
