// argus-install-kubectl is packaged only in the installation-tools image.
// Upstream installation Hooks authenticate using their own ServiceAccount;
// remote execution continues to use the separate, target-pinned kubectl.
package main

import (
	"fmt"
	"os"

	upstream "k8s.io/kubectl/pkg/cmd"
)

func main() {
	if err := upstream.NewDefaultKubectlCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
