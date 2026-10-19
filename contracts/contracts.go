// Package contracts embeds the contracts a binary applies at run time.
package contracts

import _ "embed"

//go:embed topics.yaml
var topics []byte

// Topics is contracts/topics.yaml.
func Topics() []byte {
	return topics
}
