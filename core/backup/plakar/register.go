package plakar

import (
	"github.com/opensvc/om3/v3/core/backup"
)

var (
	_ backup.Store           = &T{}
	_ backup.StreamStore     = &T{}
	_ backup.FilteredChecker = &T{}
)

func init() {
	backup.Register("plakar", New)
}

// New allocates a plakar store driver.
func New() backup.Store {
	return &T{}
}
