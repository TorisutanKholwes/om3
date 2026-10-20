package store

import (
	"fmt"
	"slices"
	"strings"

	"github.com/opensvc/om3/v3/core/driver"
	"github.com/opensvc/om3/v3/core/xconfig"
	"github.com/opensvc/om3/v3/util/key"
)

const (
	// SectionPrefix is the prefix of the cluster.conf sections declaring a
	// store, as in [store#lab].
	//
	// It is not "backup#": that section already belongs to the pushbackup
	// node action and its schedule keyword.
	SectionPrefix = "store#"
)

// Register registers the allocator of the store driver serving the type
// name, as in "type = plakar".
func Register(name string, allocator func() Store) {
	driver.Register(driver.NewID(driver.GroupStore, name), allocator)
}

// GetDriver returns a new store driver for the type name, or nil when no
// driver serves this type.
func GetDriver(name string) Store {
	i, ok := driver.GetStrict(driver.NewID(driver.GroupStore, name))
	if !ok {
		return nil
	}
	if a, ok := i.Allocator.(func() Store); ok {
		return a()
	}
	return nil
}

// SectionName returns the [store#<name>] section name of a store name, with
// or without its prefix.
func SectionName(name string) string {
	if strings.HasPrefix(name, SectionPrefix) {
		return name
	}
	return SectionPrefix + name
}

// New returns the driver of the store named name in the configuration,
// configured from its section.
//
// The driver is chosen by the type keyword of the section.
func New(config *xconfig.T, name string) (Store, error) {
	if config == nil {
		return nil, fmt.Errorf("store %s: no configuration", name)
	}
	section := SectionName(name)
	if !slices.Contains(config.SectionStrings(), section) {
		return nil, fmt.Errorf("store %s: no [%s] section in the cluster configuration", name, section)
	}
	storeType := config.GetString(key.New(section, "type"))
	if storeType == "" {
		return nil, fmt.Errorf("store %s: the type keyword is not set", name)
	}
	drv := GetDriver(storeType)
	if drv == nil {
		return nil, fmt.Errorf("store %s: no driver for type %s", name, storeType)
	}
	if i, ok := drv.(namer); ok {
		i.SetName(strings.TrimPrefix(section, SectionPrefix))
	}
	drv.SetConfig(config)
	return drv, nil
}

// List returns the names of the stores declared in the configuration.
func List(config *xconfig.T) []string {
	l := make([]string, 0)
	if config == nil {
		return l
	}
	for _, s := range config.SectionStrings() {
		if name, ok := strings.CutPrefix(s, SectionPrefix); ok {
			l = append(l, name)
		}
	}
	return l
}
