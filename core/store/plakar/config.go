package plakar

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type (
	// storesConfig is the stores.yml file of the plakar configuration
	// directory.
	storesConfig struct {
		Version string                       `yaml:"version"`
		Stores  map[string]map[string]string `yaml:"stores"`
	}
)

const (
	configVersion = "v1.0.0"

	dirPerm  = os.FileMode(0700)
	filePerm = os.FileMode(0600)
)

// writeConfig writes the plakar configuration in the private configuration
// directory: stores.yml declaring the store, with its location and no
// secret, and the sources.yml and destinations.yml plakar requires.
//
// plakar loads the three files together, and when sources.yml or
// destinations.yml is missing it falls back to its legacy configuration and
// saves the three files, which rewrites stores.yml empty.
func (t *T) writeConfig() error {
	for _, dir := range []string{t.configDir(), t.cacheDir(), t.dataDir()} {
		if err := os.MkdirAll(dir, dirPerm); err != nil {
			return fmt.Errorf("store %s: %w", t.name, err)
		}
		if err := os.Chmod(dir, dirPerm); err != nil {
			return fmt.Errorf("store %s: %w", t.name, err)
		}
	}
	options, err := t.storeOptions()
	if err != nil {
		return err
	}
	stores := storesConfig{
		Version: configVersion,
		Stores: map[string]map[string]string{
			t.name: options,
		},
	}
	b, err := yaml.Marshal(stores)
	if err != nil {
		return fmt.Errorf("store %s: %w", t.name, err)
	}
	files := map[string][]byte{
		"stores.yml":       b,
		"sources.yml":      []byte("version: " + configVersion + "\nsources: {}\n"),
		"destinations.yml": []byte("version: " + configVersion + "\ndestinations: {}\n"),
	}
	for name, content := range files {
		if err := writeFile(filepath.Join(t.configDir(), name), content); err != nil {
			return fmt.Errorf("store %s: %w", t.name, err)
		}
	}

	// Without these files, plakar prints a welcome banner on the first
	// run in a cache directory, and checks for updates over the network
	// once a day.
	for _, name := range []string{".first-run", ".disabled-security-check"} {
		p := filepath.Join(t.cacheDir(), name)
		if _, err := os.Stat(p); err == nil {
			continue
		}
		if err := writeFile(p, nil); err != nil {
			return fmt.Errorf("store %s: %w", t.name, err)
		}
	}
	return nil
}

// storeOptions returns the options of the store in stores.yml.
//
// The host key is public, and written there for the sftp package, which
// passes it to ssh as the only known host. The identity is not: it is
// loaded in a SSH agent for each command.
func (t *T) storeOptions() (map[string]string, error) {
	options := map[string]string{
		"location": t.location,
	}
	if t.hostKey != "" {
		s, err := t.hostKeyOption()
		if err != nil {
			return nil, err
		}
		options["host_key"] = s
	}
	return options, nil
}

// writeFile writes a 0600 file, through a temporary file renamed in place
// so a concurrent plakar command never reads a partial file.
func writeFile(p string, b []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(p), "."+filepath.Base(p)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(filePerm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), p)
}
