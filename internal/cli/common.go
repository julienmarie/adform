package cli

import "flag"

type commonOptions struct {
	Account   string
	Root      string
	StatePath string
	JSON      bool
	Verbose   bool
}

func bindCommonFlags(fs *flag.FlagSet, opts *commonOptions) {
	fs.StringVar(&opts.Account, "account", "", "Account/property name (<account>/meta; legacy meta/<account> supported)")
	fs.StringVar(&opts.Root, "root", ".", "Repo root")
	fs.StringVar(&opts.StatePath, "state", "", "State DB path (default .adform/state.db under root)")
	fs.BoolVar(&opts.JSON, "json", false, "Machine-readable JSON output")
	fs.BoolVar(&opts.Verbose, "verbose", false, "Verbose output")
}

func finalizeCommon(opts *commonOptions) {
	if opts.StatePath == "" {
		opts.StatePath = defaultStatePath(opts.Root)
	}
}
