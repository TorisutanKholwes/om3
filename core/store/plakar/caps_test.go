package plakar

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/opensvc/om3/v3/util/capabilities"
)

func TestCapabilitiesScanner(t *testing.T) {
	t.Run("with the binary", func(t *testing.T) {
		f := newFakePlakar(t)
		t.Setenv("PATH", f.dir)
		l, err := capabilitiesScanner(context.Background())
		require.NoError(t, err)
		require.Equal(t, []string{"drivers.store.plakar", capabilities.MakePath("plakar", f.bin())}, l)
	})
	t.Run("without the binary", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		l, err := capabilitiesScanner(context.Background())
		require.NoError(t, err)
		require.Empty(t, l)
	})
}
