package ressyncbackup

import (
	"embed"

	"github.com/opensvc/om3/v3/core/driver"
	"github.com/opensvc/om3/v3/core/keywords"
	"github.com/opensvc/om3/v3/core/manifest"
	"github.com/opensvc/om3/v3/core/naming"
	"github.com/opensvc/om3/v3/drivers/ressync"
	"github.com/opensvc/om3/v3/util/converters"
)

var (
	drvID = driver.NewID(driver.GroupSync, "backup")

	//go:embed text
	fs embed.FS

	kws = []*keywords.Keyword{
		{
			Attr:     "Store",
			Example:  "lab",
			Option:   "store",
			Required: true,
			Scopable: true,
			Text:     keywords.NewText(fs, "text/kw/store"),
		},
		{
			Attr:      "RIDs",
			Converter: converters.List,
			Example:   "fs#1 volume#1",
			Option:    "rids",
			Scopable:  true,
			Text:      keywords.NewText(fs, "text/kw/rids"),
		},
		{
			Attr:      "Exclude",
			Converter: converters.List,
			Example:   "fs#2",
			Option:    "exclude",
			Scopable:  true,
			Text:      keywords.NewText(fs, "text/kw/exclude"),
		},
		{
			Attr:     "Keep",
			Example:  "7d 4w 6m",
			Option:   "keep",
			Scopable: true,
			Text:     keywords.NewText(fs, "text/kw/keep"),
		},
	}
)

func init() {
	driver.Register(drvID, New)
}

func (t *T) DriverID() driver.ID {
	return drvID
}

// Manifest exposes to the core the input expected by the driver.
func (t *T) Manifest() *manifest.T {
	m := manifest.New(drvID, t)
	m.Kinds.Or(naming.KindSvc, naming.KindVol)
	m.Add(
		manifest.ContextObjectPath,
	)
	m.AddKeywords(ressync.BaseKeywords...)
	m.AddKeywords(kws...)
	return m
}
