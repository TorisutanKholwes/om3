package plakar

import (
	"context"
	"os/exec"

	"github.com/opensvc/om3/v3/util/capabilities"
)

func init() {
	capabilities.Register(capabilitiesScanner)
}

// capabilitiesScanner publishes the drivers.store.plakar capability and
// the path of the plakar binary, when the binary is in the PATH: without
// it, a plakar store is not usable on the node.
func capabilitiesScanner(ctx context.Context) ([]string, error) {
	l := make([]string, 0)
	bin, err := exec.LookPath(plakarBin)
	if err != nil {
		return l, nil
	}
	l = append(l, drvID.Cap())
	l = append(l, capabilities.MakePath(plakarBin, bin))
	return l, nil
}
