// Package personal holds the rule of the personal token that the generator and the dashboard share, out of the
// scopes it generates.
package personal

// Excluded are the scopes a personal token never carries, whatever the roles of its operator.
var Excluded = []string{"content:read", "content:erase", "gdpr:erase"}
