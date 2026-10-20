package resource

type (
	// RestoreOptions are the options of a "sync restore" action, passed to
	// the drivers implementing Restore.
	RestoreOptions struct {
		// Into is the directory to restore into. Empty means in place.
		Into string

		// Snapshot is the identifier of the pass to restore. Empty means
		// the most recent one.
		Snapshot string

		// RIDs are the data resources to restore, selected by --rid.
		// Empty means all.
		RIDs []string

		// Config also restores the configuration of the object.
		Config bool

		// Refs also restores the keys of the referenced sec and cfg
		// objects.
		Refs bool
	}
)
