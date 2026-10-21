package scopes

// Sensitive is the ten sensitive scopes of spec 07 §7.4.
var Sensitive = []string{"content:read", "content:write", "content:erase", "gdpr:erase", "msisdn:reveal",
	"cdr:export_bulk", "audit:read", "routing-scripts:publish", "suppressions:delete", "billing:topup"}
