package plakar

import (
	"github.com/opensvc/om3/v3/core/store"
)

var (
	_ store.Store           = &T{}
	_ store.StreamStore     = &T{}
	_ store.FilteredChecker = &T{}
)

func init() {
	store.Register("plakar", New)
}

// New allocates a plakar store driver.
func New() store.Store {
	return &T{}
}
