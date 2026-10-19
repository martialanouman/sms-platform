// Command netpol renders deploy/k8s/networkpolicies.yaml from the manifests beside it.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/martialanouman/sms-platform/internal/netpol"
)

const dir = "deploy/k8s"

func main() {
	out, err := netpol.Render(os.DirFS(dir))
	if err == nil {
		err = os.WriteFile(filepath.Join(dir, netpol.File), out, 0o600)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
