// Package smsplatform is the root of the SMS gateway and its operations dashboard.
package smsplatform

//go:generate go tool buf generate
//go:generate go run ./tools/catalog -doc docs/topics.md
//go:generate go run ./tools/openapi -gen internal/platform/scopes/scopes.gen.go
//go:generate go run ./tools/grants
//go:generate go tool sqlc generate
//go:generate go run ./tools/netpol
