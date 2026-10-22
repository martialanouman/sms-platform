package http

import (
	"context"
	"errors"
	"strings"

	"github.com/martialanouman/sms-platform/internal/dashboard"
)

const (
	unknownOperator = "Cet opérateur est inconnu."
	unknownRole     = "Ce rôle est inconnu."
	selfLockout     = "Ce changement vous retirerait la gestion des opérateurs ou des rôles."
)

func actor(ctx context.Context) string {
	c, _ := ctx.Value(currentKey{}).(dashboard.Current)
	return c.OperatorID
}

func operatorDTO(o dashboard.OperatorRecord) Operator {
	dto := Operator{Id: o.ID, Email: o.Email, DisplayName: o.DisplayName, Status: OperatorStatusDisabled, Roles: make([]RoleRef, len(o.Roles)),
		SecondFactorEnrolled: o.SecondFactor}
	if o.Active {
		dto.Status = OperatorStatusActive
	}
	for i, r := range o.Roles {
		dto.Roles[i] = RoleRef{Id: r.ID, Name: r.Name}
	}
	if o.Link != nil {
		dto.AccessLink = &AccessLink{Kind: AccessLinkKind(o.Link.Kind), State: AccessLinkState(o.Link.State)}
	}
	return dto
}

func roleDTO(r dashboard.RoleRecord) Role {
	return Role{Id: r.ID, Name: r.Name, Description: r.Description, IsDefault: r.Default, Permissions: r.Permissions, Holders: r.Holders}
}

func forbidden(permission string) ForbiddenJSONResponse {
	return ForbiddenJSONResponse{Code: ErrorCodeForbiddenScope, Message: "Il faut la permission " + permission + "."}
}

func invalid(err error) (ValidationErrorJSONResponse, bool) {
	var field dashboard.InvalidField
	if !errors.As(err, &field) {
		return ValidationErrorJSONResponse{}, false
	}
	return ValidationErrorJSONResponse{Code: ErrorCodeValidationError, Message: "La requête est mal formée.",
		Errors: &[]fieldError{{Field: field.Field, Message: "inconnu"}}}, true
}

func (s server) ListOperators(ctx context.Context, _ ListOperatorsRequestObject) (ListOperatorsResponseObject, error) {
	all, err := s.Service.Operators(ctx)
	out := make(ListOperators200JSONResponse, len(all))
	for i, o := range all {
		out[i] = operatorDTO(o)
	}
	return out, err
}

func (s server) ListAuditOperators(ctx context.Context, _ ListAuditOperatorsRequestObject) (ListAuditOperatorsResponseObject, error) {
	all, err := s.Service.Operators(ctx)
	out := make(ListAuditOperators200JSONResponse, len(all))
	for i, o := range all {
		d := operatorDTO(o)
		out[i] = OperatorRef{Id: d.Id, Email: d.Email, DisplayName: d.DisplayName, Status: OperatorRefStatus(d.Status)}
	}
	return out, err
}

func (s server) CreateOperator(ctx context.Context, request CreateOperatorRequestObject) (CreateOperatorResponseObject, error) {
	created, err := s.Service.CreateOperator(ctx, actor(ctx), request.Body.Email, request.Body.DisplayName)
	if refused, ok := invalid(err); ok {
		return CreateOperator422JSONResponse{refused}, nil
	}
	switch {
	case errors.Is(err, dashboard.ErrEmailTaken):
		return CreateOperator409JSONResponse{Code: ErrorCodeEmailTaken, Message: "Un opérateur a déjà cette adresse."}, nil
	case err != nil:
		return nil, err
	}
	return CreateOperator201JSONResponse(operatorDTO(created)), nil
}

func (s server) UpdateOperator(ctx context.Context, request UpdateOperatorRequestObject) (UpdateOperatorResponseObject, error) {
	updated, err := s.Service.SetOperatorStatus(ctx, actor(ctx), request.OperatorId, request.Body.Status == OperatorUpdateStatusActive)
	switch {
	case errors.Is(err, dashboard.ErrForbidden):
		return UpdateOperator403JSONResponse{forbidden("operators:write")}, nil
	case errors.Is(err, dashboard.ErrNotFound):
		return UpdateOperator404JSONResponse{NotFoundJSONResponse{Code: ErrorCodeNotFound, Message: unknownOperator}}, nil
	case errors.Is(err, dashboard.ErrSelfLockout):
		return UpdateOperator409JSONResponse{SelfLockoutJSONResponse{Code: ErrorCodeSelfLockout, Message: "Vous ne pouvez pas vous désactiver."}}, nil
	case err != nil:
		return nil, err
	}
	return UpdateOperator200JSONResponse(operatorDTO(updated)), nil
}

func (s server) SetOperatorRoles(ctx context.Context, request SetOperatorRolesRequestObject) (SetOperatorRolesResponseObject, error) {
	updated, err := s.Service.SetOperatorRoles(ctx, actor(ctx), request.OperatorId, request.Body.RoleIds)
	if refused, ok := invalid(err); ok {
		return SetOperatorRoles422JSONResponse{refused}, nil
	}
	switch {
	case errors.Is(err, dashboard.ErrForbidden):
		return SetOperatorRoles403JSONResponse{forbidden("operators:write")}, nil
	case errors.Is(err, dashboard.ErrNotFound):
		return SetOperatorRoles404JSONResponse{NotFoundJSONResponse{Code: ErrorCodeNotFound, Message: unknownOperator}}, nil
	case errors.Is(err, dashboard.ErrSelfLockout):
		return SetOperatorRoles409JSONResponse{SelfLockoutJSONResponse{Code: ErrorCodeSelfLockout, Message: selfLockout}}, nil
	case err != nil:
		return nil, err
	}
	return SetOperatorRoles200JSONResponse(operatorDTO(updated)), nil
}

func (s server) RequestOperatorAccessLink(ctx context.Context, request RequestOperatorAccessLinkRequestObject) (RequestOperatorAccessLinkResponseObject, error) {
	err := s.Service.RequestAccessLink(ctx, actor(ctx), request.OperatorId)
	switch {
	case errors.Is(err, dashboard.ErrForbidden):
		return RequestOperatorAccessLink403JSONResponse{forbidden("operators:write")}, nil
	case errors.Is(err, dashboard.ErrNotFound):
		return RequestOperatorAccessLink404JSONResponse{NotFoundJSONResponse{Code: ErrorCodeNotFound, Message: unknownOperator}}, nil
	case errors.Is(err, dashboard.ErrOperatorDisabled):
		return RequestOperatorAccessLink409JSONResponse{Code: ErrorCodeOperatorDisabled, Message: "Aucun lien ne part vers un compte désactivé."}, nil
	case err != nil:
		return nil, err
	}
	return RequestOperatorAccessLink202Response{}, nil
}

func (s server) ListRoles(ctx context.Context, _ ListRolesRequestObject) (ListRolesResponseObject, error) {
	all, err := s.Service.Roles(ctx)
	out := make(ListRoles200JSONResponse, len(all))
	for i, r := range all {
		out[i] = roleDTO(r)
	}
	return out, err
}

func (s server) CreateRole(ctx context.Context, request CreateRoleRequestObject) (CreateRoleResponseObject, error) {
	created, err := s.Service.CreateRole(ctx, actor(ctx), request.Body.Name, request.Body.Description, request.Body.Permissions)
	if refused, ok := invalid(err); ok {
		return CreateRole422JSONResponse{refused}, nil
	}
	switch {
	case errors.Is(err, dashboard.ErrForbidden):
		return CreateRole403JSONResponse{forbidden("roles:write")}, nil
	case errors.Is(err, dashboard.ErrRoleNameTaken):
		return CreateRole409JSONResponse{Code: ErrorCodeRoleNameTaken, Message: "Un rôle porte déjà ce nom."}, nil
	case err != nil:
		return nil, err
	}
	return CreateRole201JSONResponse(roleDTO(created)), nil
}

func (s server) UpdateRole(ctx context.Context, request UpdateRoleRequestObject) (UpdateRoleResponseObject, error) {
	updated, err := s.Service.UpdateRole(ctx, actor(ctx), request.RoleId, request.Body.Description, request.Body.Permissions)
	if refused, ok := invalid(err); ok {
		return UpdateRole422JSONResponse{refused}, nil
	}
	switch {
	case errors.Is(err, dashboard.ErrForbidden):
		return UpdateRole403JSONResponse{forbidden("roles:write")}, nil
	case errors.Is(err, dashboard.ErrNotFound):
		return UpdateRole404JSONResponse{NotFoundJSONResponse{Code: ErrorCodeNotFound, Message: unknownRole}}, nil
	case errors.Is(err, dashboard.ErrRoleIsDefault):
		return UpdateRole409JSONResponse{Code: ErrorCodeRoleIsDefault, Message: "Un rôle par défaut ne se modifie pas : il est rétabli à chaque déploiement."}, nil
	case errors.Is(err, dashboard.ErrSelfLockout):
		return UpdateRole409JSONResponse{Code: ErrorCodeSelfLockout, Message: selfLockout}, nil
	case err != nil:
		return nil, err
	}
	return UpdateRole200JSONResponse(roleDTO(updated)), nil
}

func (s server) DeleteRole(ctx context.Context, request DeleteRoleRequestObject) (DeleteRoleResponseObject, error) {
	err := s.Service.DeleteRole(ctx, actor(ctx), request.RoleId)
	var held dashboard.RoleHeld
	switch {
	case errors.As(err, &held):
		return DeleteRole409JSONResponse{Code: ErrorCodeRoleHeld, Message: "Ce rôle est détenu par " + strings.Join(held.Holders, ", ") + "."}, nil
	case errors.Is(err, dashboard.ErrForbidden):
		return DeleteRole403JSONResponse{forbidden("roles:write")}, nil
	case errors.Is(err, dashboard.ErrNotFound):
		return DeleteRole404JSONResponse{NotFoundJSONResponse{Code: ErrorCodeNotFound, Message: unknownRole}}, nil
	case errors.Is(err, dashboard.ErrRoleIsDefault):
		return DeleteRole409JSONResponse{Code: ErrorCodeRoleIsDefault, Message: "Un rôle par défaut ne se supprime pas."}, nil
	case err != nil:
		return nil, err
	}
	return DeleteRole204Response{}, nil
}
