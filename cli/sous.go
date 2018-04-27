package cli

import (
	"github.com/opentable/sous/graph"
	"github.com/opentable/sous/util/cmdr"
	"github.com/opentable/sous/util/whitespace"
	"github.com/samsalisbury/semv"
)

// Sous is the main sous command.
type Sous struct {
	// CLI is a reference to the CLI singleton.
	CLI *CLI
	graph.LogSink
	// Version is the version of Sous itself.
	Version semv.Version
	// OS is the OS this Sous is running on.
	OS string `inject:"optional"`
	// Arch is the architecture this Sous is running on.
	Arch string `inject:"optional"`
	// GoVersion is the version of Go this sous was built with.
	GoVersion string `inject:"optional"`
	// flags holds the values of flags passed to this command
	flags struct {
		Help bool
	}
}

// TopLevelCommands is populated once per command file (beginning sous_) in this
// directory.
var TopLevelCommands = cmdr.Commands{}

const sousHelp = `sous is a tool to help speed up the build/test/deploy cycle at your organisation

usage: sous <command>

sous helps by automating the boring bits of the build/test/deploy cycle. It
provides commands in this CLI for performing all the actions the sous server is
capable of, like building container images, testing them, and instigating
deployments.

sous also has some extra convenience commands for doing things like getting free
ports and host names, managing its own configuration, and spinning up
subsections of your production environment locally, for easy testing.

For a list of commands, use 'sous help'

Please report any issue with sous to https://github.com/opentable/sous/issues
pull requests are welcome.
`

// Help returns the top-level help for Sous.
func (*Sous) Help() string { return sousHelp }

// Execute exists to present a helpful error to the user, in the case they just
// run 'sous' with not subcommand.
func (s *Sous) Execute(args []string) cmdr.Result {
	r := s.CLI.InvokeWithoutPrinting([]string{"sous", "help"})
	success, ok := r.(*cmdr.SuccessResult)
	if !ok {
		return s.usage()
	}
	return cmdr.UsageErrorf(whitespace.Trim(success.String()) + "\n")
}

func (s *Sous) usage() cmdr.ErrorResult {
	err := cmdr.UsageErrorf("usage: sous [options] command")
	err.Tip = "try `sous help` for a list of commands"
	return err
}

// Subcommands returns all the top-level sous subcommands.
func (s *Sous) Subcommands() cmdr.Commands {
	return TopLevelCommands
}
