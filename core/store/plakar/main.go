// Package plakar is the plakar store driver of the om3 store framework.
//
// It embeds no plakar library: it runs the plakar binary installed on the
// node, which keeps om3 independent of the plakar versions. The bytes of the
// backed up paths and streams go from the node to the Kloset store through
// that binary, never through the om3 api.
//
// Every command is prefixed by
//
//	plakar -configdir <dir> -cachedir <dir> -datadir <dir> -keyfile <file> at @<name>
//
// the three directories being private to the driver, under the om3 var
// directory, and the key file holding the passphrase being maintained by om3
// under its run directory.
package plakar

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/opensvc/om3/v3/core/datarecv"
	"github.com/opensvc/om3/v3/core/driver"
	"github.com/opensvc/om3/v3/core/naming"
	"github.com/opensvc/om3/v3/core/rawconfig"
	"github.com/opensvc/om3/v3/core/store"
	"github.com/opensvc/om3/v3/core/xconfig"
	"github.com/opensvc/om3/v3/util/capabilities"
	"github.com/opensvc/om3/v3/util/key"
	"github.com/opensvc/om3/v3/util/plog"
)

type (
	// T is a plakar store, declared by a [store#<name>] section of type
	// plakar.
	T struct {
		name string

		// location is the Kloset store location, a local path or a
		// sftp:// or s3:// url.
		location string

		// passphrase, identity and hostKey are key references, as in
		// "from store/sec/lab key passphrase".
		passphrase string
		identity   string
		hostKey    string

		// keep is the default retention of the resources using this
		// store.
		keep store.Retention

		// configErr is the error SetConfig met, returned by every
		// operation: a store with an invalid configuration, or on a
		// node without the plakar binary, is not usable.
		configErr error

		log *plog.Logger

		// bin is the path of the plakar binary.
		bin string

		// root is the parent directory of the private directories.
		root string

		// runDir is the directory of the key file.
		runDir string

		// installKey installs the passphrase key file, installKeyFile
		// unless a test replaces it.
		installKey func() error

		// keyDecoder, if set, replaces the datastore decoding of the
		// key references, for tests.
		keyDecoder func(option, ref string) ([]byte, error)

		// prepared is set once the store configuration is written and
		// the store is known to exist.
		prepared   bool
		preparedMu sync.Mutex
	}
)

const (
	plakarBin = "plakar"
)

var (
	drvID = driver.NewID(driver.GroupStore, "plakar")

	// ErrNoBinary is returned when the plakar binary is not found on the
	// node.
	ErrNoBinary = errors.New("the plakar binary is not installed on this node")
)

// Name returns the name of the store, the part of the section name after
// "store#".
func (t *T) Name() string {
	return t.name
}

// SetName sets the name of the store.
func (t *T) SetName(s string) {
	t.name = s
}

// SetLogger sets the logger of the store commands.
func (t *T) SetLogger(l *plog.Logger) {
	t.log = l
}

// Keep returns the retention the store declares, the default of the
// resources using it.
func (t *T) Keep() store.Retention {
	return t.keep
}

// SetConfig loads the store configuration from its section and writes the
// plakar configuration files in the private configuration directory.
//
// The store itself is created on the first operation needing it, if it does
// not exist yet, as SetConfig has no context to run a command with.
func (t *T) SetConfig(config *xconfig.T) {
	t.configErr = t.setConfig(config)
	if t.configErr != nil {
		return
	}
	t.configErr = t.writeConfig()
}

func (t *T) setConfig(config *xconfig.T) error {
	if t.name == "" {
		return fmt.Errorf("plakar store: no name")
	}
	section := store.SectionName(t.name)
	get := func(option string) string {
		return config.GetString(key.New(section, option))
	}
	t.location = get("store")
	t.passphrase = get("passphrase")
	t.identity = get("identity")
	t.hostKey = get("host_key")
	if t.location == "" {
		return fmt.Errorf("store %s: the store keyword is not set", t.name)
	}
	if t.passphrase == "" {
		return fmt.Errorf("store %s: the passphrase keyword is not set", t.name)
	}
	keep, err := store.ParseRetention(get("keep"))
	if err != nil {
		return fmt.Errorf("store %s: keep: %w", t.name, err)
	}
	t.keep = keep
	if t.root == "" {
		t.root = filepath.Join(rawconfig.Paths.Var, "store", t.name, plakarBin)
	}
	if t.runDir == "" {
		t.runDir = filepath.Join(rawconfig.Paths.Run, "store", t.name)
	}
	if t.log == nil {
		t.log = plog.NewDefaultLogger().Attr("store", t.name).WithPrefix(fmt.Sprintf("store: %s: ", t.name))
	}
	if t.installKey == nil {
		t.installKey = t.installKeyFile
	}
	if t.bin == "" {
		bin, err := lookPath()
		if err != nil {
			return fmt.Errorf("store %s: %w", t.name, err)
		}
		t.bin = bin
	}
	return nil
}

// lookPath returns the path of the plakar binary the capabilities scanner
// found, or the one found in the PATH.
func lookPath() (string, error) {
	if p := capabilities.GetPath(plakarBin); p != "" {
		return p, nil
	}
	p, err := exec.LookPath(plakarBin)
	if err != nil {
		return "", ErrNoBinary
	}
	return p, nil
}

func (t *T) configDir() string {
	return filepath.Join(t.root, "config")
}

func (t *T) cacheDir() string {
	return filepath.Join(t.root, "cache")
}

func (t *T) dataDir() string {
	return filepath.Join(t.root, "data")
}

func (t *T) keyFile() string {
	return filepath.Join(t.runDir, "passphrase")
}

// installKeyFile installs the passphrase key in the key file, or refreshes
// it if the secret changed.
//
// The reference is declared in the cluster configuration, where only root
// writes, so the key is read with root privileges.
func (t *T) installKeyFile() error {
	km, err := datarecv.ParseKeyMetaRel(t.passphrase, naming.NsRoot)
	if err != nil {
		return fmt.Errorf("store %s: passphrase: %w", t.name, err)
	}
	if err := km.RootCacheFileAt(t.keyFile()); err != nil {
		return fmt.Errorf("store %s: passphrase: %w", t.name, err)
	}
	return nil
}

// decodeKey returns the content of a key reference of the store section.
func (t *T) decodeKey(option, ref string) ([]byte, error) {
	if t.keyDecoder != nil {
		return t.keyDecoder(option, ref)
	}
	return t.rootDecodeKey(option, ref)
}

func (t *T) rootDecodeKey(option, ref string) ([]byte, error) {
	km, err := datarecv.ParseKeyMetaRel(ref, naming.NsRoot)
	if err != nil {
		return nil, fmt.Errorf("store %s: %s: %w", t.name, option, err)
	}
	b, err := km.RootDecode()
	if err != nil {
		return nil, fmt.Errorf("store %s: %s: %w", t.name, option, err)
	}
	return b, nil
}
