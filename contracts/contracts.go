// Package contracts embeds the contracts a binary applies at run time.
package contracts

import (
	"embed"
	"io/fs"
)

//go:embed topics.yaml
var topics []byte

// Topics is contracts/topics.yaml.
func Topics() []byte {
	return topics
}

//go:embed openapi-bff.yaml openapi-admin.yaml
var openapi embed.FS

// OpenAPI holds the BFF contract and the Admin contract it refers to, which the dashboard validates its requests
// against.
func OpenAPI() fs.FS {
	return openapi
}
