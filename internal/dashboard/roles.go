package dashboard

import (
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
			[]string{"routing-scripts:publish", "msisdn:reveal", "cdr:export_bulk", "sessions:read", "sessions:write", "stream:read"},
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
