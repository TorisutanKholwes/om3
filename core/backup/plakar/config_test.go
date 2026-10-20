package plakar

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/opensvc/om3/v3/core/backup"
	"github.com/opensvc/om3/v3/core/object"
	"github.com/opensvc/om3/v3/core/rawconfig"
	"github.com/opensvc/om3/v3/core/xconfig"
)

// nodeConfig returns the merged node configuration, as the one a store is
// configured from, with the config data.
func nodeConfig(t *testing.T, data string) *xconfig.T {
	t.Helper()
	n, err := object.NewNode(object.WithConfigData([]byte(data)), object.WithVolatile(true))
	require.NoError(t, err)
	return n.MergedConfig()
}

func TestNewFromConfig(t *testing.T) {
	f := newFakePlakar(t)
	t.Setenv("PATH", f.dir+":"+os.Getenv("PATH"))
	root := t.TempDir()
	defer rawconfig.ReloadForTest(root)()

	cfg := nodeConfig(t, `
[store#lab]
type = plakar
store = sftp://backup@store.lab/plakar/om3
passphrase = from backup/sec/lab key passphrase
keep = 7d 4w 6m
`)
	require.Equal(t, []string{"lab"}, backup.List(cfg))

	drv, err := backup.New(cfg, "lab")
	require.NoError(t, err)
	s := drv.(*T)
	require.NoError(t, s.configErr)
	require.Equal(t, "lab", s.Name())
	require.Equal(t, "sftp://backup@store.lab/plakar/om3", s.location)
	require.Equal(t, backup.Retention{Days: 7, Weeks: 4, Months: 6}, s.Keep())
	require.Equal(t, f.bin(), s.bin)
	require.Equal(t, filepath.Join(rawconfig.Paths.Var, "store", "lab", "plakar", "config"), s.configDir())
	_, err = os.Stat(filepath.Join(s.configDir(), "stores.yml"))
	require.NoError(t, err)
}

func TestSetConfigErrors(t *testing.T) {
	root := t.TempDir()
	defer rawconfig.ReloadForTest(root)()
	for title, c := range map[string]string{
		"no store":      "[store#lab]\ntype = plakar\npassphrase = from backup/sec/lab key passphrase\n",
		"no passphrase": "[store#lab]\ntype = plakar\nstore = /srv/kloset\n",
		"invalid keep":  "[store#lab]\ntype = plakar\nstore = /srv/kloset\npassphrase = from backup/sec/lab key passphrase\nkeep = 7x\n",
	} {
		t.Run(title, func(t *testing.T) {
			cfg := nodeConfig(t, c)
			drv, err := backup.New(cfg, "lab")
			require.NoError(t, err)
			require.Error(t, drv.(*T).configErr)
		})
	}
	t.Run("no binary", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		cfg := nodeConfig(t, "[store#lab]\ntype = plakar\nstore = /srv/kloset\npassphrase = from backup/sec/lab key passphrase\n")
		drv, err := backup.New(cfg, "lab")
		require.NoError(t, err)
		require.ErrorIs(t, drv.(*T).configErr, ErrNoBinary)
	})
	t.Run("no section", func(t *testing.T) {
		cfg := nodeConfig(t, "")
		_, err := backup.New(cfg, "lab")
		require.Error(t, err)
	})
}

func TestRegistered(t *testing.T) {
	require.IsType(t, &T{}, backup.GetDriver("plakar"))
}
