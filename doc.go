// Package smsplatform is the root of the SMS gateway and its operations dashboard.
package smsplatform

//go:generate go tool buf generate
//go:generate go run ./tools/catalog -doc docs/topics.md
//go:generate go run ./tools/openapi -gen internal/platform/scopes/scopes.gen.go
//go:generate go run ./tools/grants
//go:generate go tool sqlc generate
//go:generate go tool oapi-codegen -config internal/dashboard/http/oapi-codegen.yaml contracts/openapi-bff.yaml
//go:generate pnpm -C web exec openapi-typescript ../contracts/openapi-bff.yaml -o src/api.gen.ts
//go:generate pnpm -C web exec tsr generate
//go:generate go run ./tools/netpol
