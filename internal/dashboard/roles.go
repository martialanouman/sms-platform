package dashboard

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/martialanouman/sms-platform/internal/platform/scopes"
)

// Owner is the default role that holds every permission, posed on the bootstrap operator.
const Owner = "Propriétaire"

// Role is a named set of Admin API scopes and BFF permissions.
type Role struct {
	Name        string
	Description string
	Permissions []string
}

var sensitive = []string{"content:read", "content:erase", "gdpr:erase", "msisdn:reveal", "cdr:export_bulk",
	"audit:read", "routing-scripts:publish", "suppressions:delete", "billing:topup"}

// RequiresMFA is whether held carries more than ordinary reads (spec 08 §8.3): a write, a sensitive scope of spec 07
// §7.4 or a BFF permission.
func RequiresMFA(held []string) bool {
	return slices.ContainsFunc(held, func(p string) bool {
		return strings.HasSuffix(p, ":write") || slices.Contains(sensitive, p) || slices.Contains(scopes.Permissions, p)
	})
}

func suffixed(suffix string, tags ...string) []string {
	out := make([]string, len(tags))
	for i, tag := range tags {
		out[i] = tag + suffix
	}
	return out
}

func readWrite(tags ...string) []string {
	return slices.Concat(suffixed(":read", tags...), suffixed(":write", tags...))
}

func permissions(parts ...[]string) []string {
	out := slices.Concat(parts...)
	slices.Sort(out)
	return slices.Compact(out)
}

// DefaultRoles are the roles of spec 08 §8.5, as step 012a arbitrage 145 reads them.
func DefaultRoles() []Role {
	accounts := []string{"accounts", "customers", "customer-groups", "sender-ids"}
	support := suffixed(":read", slices.Concat(accounts, []string{"routes", "exact-routes", "connectors", "smpp-sessions", "cdr",
		"billing", "traffic"})...)
	var noc []string
	for _, s := range scopes.All {
		if strings.HasSuffix(s, ":read") && s != "audit:read" && s != "content:read" {
			noc = append(noc, s)
		}
	}
	return []Role{
		{Owner, "Tout, la gestion des opérateurs et des rôles comprise.", permissions(scopes.All, scopes.Permissions)},
		{"Exploitation", "Écrit le routage, les numéros exacts, les connecteurs, les sessions, l'anti-spam, les scripts et " +
			"leur publication, la réécriture, les numéros entrants, l'opt-out sans le lever, les dead-letters et la " +
			"plateforme ; lit le reste.", permissions(
			readWrite("routes", "exact-routes", "connectors", "smpp-sessions", "antispam", "routing-scripts", "sender-rewrite",
				"inbound-numbers", "suppressions", "opt-out-keywords", "dead-letters", "platform"),
			[]string{"routing-scripts:publish", "msisdn:reveal", "cdr:export_bulk", "stream:read"},
			suffixed(":read", slices.Concat(accounts, []string{"credentials", "billing", "cdr", "jobs", "traffic"})...))},
		{"Scripts", "Écrit les scripts de routage sans les publier ; lit le trafic.",
			permissions([]string{"routing-scripts:read", "routing-scripts:write", "traffic:read", "stream:read"})},
		{"Support", "Lit les comptes, le routage, les connecteurs, les sessions, les CDR, la trace et la facturation, " +
			"MSISDN en clair ; jamais le corps, ni les scripts, ni la réécriture.",
			permissions(support, []string{"msisdn:reveal", "stream:read"})},
		{"Finance", "Tient la facturation ; lit ailleurs comme Support, sans MSISDN en clair.",
			permissions(support, []string{"billing:write", "billing:topup", "stream:read"})},
		{"Reporting", "Lit la facturation et le trafic.", permissions([]string{"billing:read", "traffic:read", "stream:read"})},
		{"Clientèle", "Écrit les clients, les comptes, les identifiants, les groupes et la facturation, sans recharger.",
			permissions(readWrite(slices.Concat(accounts, []string{"credentials"})...), []string{"billing:read", "billing:write"})},
		{"Conformité", "Tient l'opt-out et le lève, efface au titre du RGPD et détruit un contenu ; lit les numéros " +
			"entrants, les comptes et les CDR, MSISDN en clair et export de masse.", permissions(
			readWrite("suppressions", "opt-out-keywords"), suffixed(":read", slices.Concat(accounts, []string{"inbound-numbers", "cdr"})...),
			[]string{"suppressions:delete", "gdpr:erase", "content:erase", "msisdn:reveal", "cdr:export_bulk"})},
		{"Audit", "Lit le journal d'audit, seul.", []string{"audit:read"}},
		{"noc", "Lit tout, temps réel compris, sans rien écrire.", permissions(noc, []string{"stream:read"})},
	}
}

var (
	// ErrRoleNameTaken is a name a role already has, case-insensitively.
	ErrRoleNameTaken = errors.New("role name taken")
	// ErrRoleIsDefault is a change to a default role, which every start reseeds.
	ErrRoleIsDefault = errors.New("role is default")
)

// RoleHeld refuses to delete a role its Holders still hold.
type RoleHeld struct {
	Holders []string
}

func (e RoleHeld) Error() string {
	return "role held by " + strings.Join(e.Holders, ", ")
}

// RoleRecord is a role as the BFF lists it, with the emails of its holders.
type RoleRecord struct {
	Role
	ID      string
	Default bool
	Holders []string
}

// Roles lists every role, default and custom, by name.
func (s Service) Roles(ctx context.Context) ([]RoleRecord, error) {
	var all []RoleRecord
	err := s.Store.Tx(ctx, func(tx Tx) (err error) {
		all, err = tx.Roles(ctx, "")
		return err
	})
	return all, err
}

func role(ctx context.Context, tx Tx, id string) (RoleRecord, error) {
	all, err := tx.Roles(ctx, id)
	switch {
	case err != nil:
		return RoleRecord{}, err
	case len(all) == 0:
		return RoleRecord{}, ErrNotFound
	}
	return all[0], nil
}

func generated(held []string) ([]string, error) {
	for i, p := range held {
		if !slices.Contains(scopes.All, p) && !slices.Contains(scopes.Permissions, p) {
			return nil, InvalidField{Field: fmt.Sprintf("permissions[%d]", i)}
		}
	}
	return append([]string{}, permissions(held)...), nil
}

// CreateRole composes a custom role from the generated permissions.
func (s Service) CreateRole(ctx context.Context, actor, name, description string, held []string) (RoleRecord, error) {
	name, description = strings.TrimSpace(name), strings.TrimSpace(description)
	if name == "" {
		return RoleRecord{}, InvalidField{Field: "name"}
	}
	held, err := generated(held)
	if err != nil {
		return RoleRecord{}, err
	}
	var created RoleRecord
	err = s.Store.Tx(ctx, func(tx Tx) error {
		if err := authorize(ctx, tx, actor, "roles:write"); err != nil {
			return err
		}
		id, ok, err := tx.InsertRole(ctx, Role{Name: name, Description: description, Permissions: held})
		switch {
		case err != nil:
			return err
		case !ok:
			return ErrRoleNameTaken
		}
		if err := tx.Log(ctx, Entry{Actor: actor, Event: EventRoleCreated, Target: id, Success: true}); err != nil {
			return err
		}
		created, err = role(ctx, tx, id)
		return err
	})
	return created, err
}

func custom(ctx context.Context, tx Tx, id string) error {
	isDefault, found, err := tx.RoleDefault(ctx, id)
	switch {
	case err != nil:
		return err
	case !found:
		return ErrNotFound
	case isDefault:
		return ErrRoleIsDefault
	}
	return nil
}

// UpdateRole replaces the description and the permissions of a custom role; nobody loses operators:write or
// roles:write by his own hand.
func (s Service) UpdateRole(ctx context.Context, actor, id, description string, held []string) (RoleRecord, error) {
	description = strings.TrimSpace(description)
	held, err := generated(held)
	if err != nil {
		return RoleRecord{}, err
	}
	var updated RoleRecord
	err = s.Store.Tx(ctx, func(tx Tx) error {
		if err := authorize(ctx, tx, actor, "roles:write"); err != nil {
			return err
		}
		if err := custom(ctx, tx, id); err != nil {
			return err
		}
		current, err := role(ctx, tx, id)
		if err != nil {
			return err
		}
		if current.Description != description || !slices.Equal(current.Permissions, held) {
			if err := guard(ctx, tx, actor, func() error { return tx.UpdateRole(ctx, id, description, held) }); err != nil {
				return err
			}
			if err := tx.Log(ctx, Entry{Actor: actor, Event: EventRoleUpdated, Target: id, Success: true}); err != nil {
				return err
			}
		}
		updated, err = role(ctx, tx, id)
		return err
	})
	return updated, err
}

// DeleteRole deletes a custom role nobody holds.
func (s Service) DeleteRole(ctx context.Context, actor, id string) error {
	return s.Store.Tx(ctx, func(tx Tx) error {
		if err := authorize(ctx, tx, actor, "roles:write"); err != nil {
			return err
		}
		if err := custom(ctx, tx, id); err != nil {
			return err
		}
		current, err := role(ctx, tx, id)
		switch {
		case err != nil:
			return err
		case len(current.Holders) > 0:
			return RoleHeld{Holders: current.Holders}
		}
		if err := tx.DeleteRole(ctx, id); err != nil {
			return err
		}
		return tx.Log(ctx, Entry{Actor: actor, Event: EventRoleDeleted, Target: id, Success: true})
	})
}
