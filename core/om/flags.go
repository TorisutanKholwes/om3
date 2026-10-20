package om

import (
	// Necessary to use go:embed
	_ "embed"

	"github.com/spf13/pflag"

	"github.com/opensvc/om3/v3/core/commoncmd"
	commands "github.com/opensvc/om3/v3/core/omcmd"
)

func addFlagsGlobal(flags *pflag.FlagSet, options *commands.OptsGlobal) {
	commoncmd.AddFlagsGlobal(flags, &options.OptsGlobal)
}

func flagLocal(flags *pflag.FlagSet, p *bool) {
	flags.BoolVar(p, "local", false, "inline action on local instance")
}

func flagStonithNode(flags *pflag.FlagSet, p *string) {
	flags.StringVar(p, "node", "", "the cluster node to fence")
}

func addFlagMonitor(flags *pflag.FlagSet, p *bool) {
	flags.BoolVarP(p, "monitor", "m", false, "refresh only the monitored resources in the cached instance status data")
}

func addFlagObject(flags *pflag.FlagSet, p *string) {
	flags.StringVarP(p, "service", "", "", "execute on a list of objects")
	flags.StringVarP(p, "selector", "s", "", "execute on a list of objects")
	flags.MarkHidden("service")
}

func hiddenFlagLocal(flags *pflag.FlagSet, p *bool) {
	flagLocal(flags, p)
	flags.Lookup("local").Hidden = true
}

func addFlagsSyncRestore(flags *pflag.FlagSet, options *commands.CmdObjectInstanceRestore) {
	flags.StringVar(&options.Into, "into", "", "restore into this directory, one sub-directory per resource, instead of in place")
	flags.StringVar(&options.Snapshot, "snapshot", "", "restore the pass with this identifier, instead of the latest snapshots")
	flags.BoolVar(&options.Config, "config", false, "also restore the object configuration")
	flags.BoolVar(&options.Refs, "refs", false, "also restore the keys of the referenced sec and cfg objects")
	flags.BoolVar(&options.Stop, "stop", false, "stop the instance before an in place restore")
}
