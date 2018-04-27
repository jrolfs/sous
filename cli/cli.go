// Package cli implements the Sous Command Line Interface. It is a
// presentation layer, and contains no core logic.
package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/opentable/sous/config"
	"github.com/opentable/sous/graph"
	sous "github.com/opentable/sous/lib"
	"github.com/opentable/sous/util/cmdr"
	"github.com/opentable/sous/util/logging"
	"github.com/opentable/sous/util/logging/messages"
	"github.com/opentable/sous/util/yaml"
	"github.com/pkg/errors"
	"github.com/samsalisbury/psyringe/experiment"
	"github.com/samsalisbury/semv"
)

// GeneralErrorf is an convenience method for returning from commands.
func GeneralErrorf(traceID sous.TraceID, format string, a ...interface{}) cmdr.ErrorResult {
	return EnsureErrorResult(fmt.Errorf(format, a...), traceID)
}

// EnsureErrorResult is a convenience for returning from a command.
func EnsureErrorResult(err error, traceID sous.TraceID) cmdr.ErrorResult {
	var ls logging.LogSink
	ls = logging.Log
	messages.ReportLogFieldsMessage("Error:", logging.DebugLevel, ls, err)
	result := cmdr.EnsureErrorResult(err)
	result.SetTraceID(traceID)
	return result
}

// ProduceResult converts errors into Results
func ProduceResult(err error, traceID sous.TraceID) cmdr.Result {
	if err != nil {
		return EnsureErrorResult(err, traceID)
	}

	return cmdr.Success()
}

type (
	// CLI describes the command line interface for Sous
	CLI struct {
		*cmdr.CLI
		LogSink logging.LogSink
		graph   *graph.SousGraph
		TraceID sous.TraceID
	}
	// Addable objects are able to receive lists of interface{}, presumably to add
	// them to a DI registry. Abstracts Psyringe's Add()
	Addable interface {
		Add(...interface{})
	}

	// A Registrant is able to add values to an Addable (implicitly: a Psyringe)
	Registrant interface {
		RegisterOn(Addable)
	}
)

// SuccessYAML lets you return YAML on the command line.
func SuccessYAML(v interface{}) cmdr.Result {
	b, err := yaml.Marshal(v)
	if err != nil {
		return cmdr.InternalErrorf("unable to marshal YAML: %s", err)
	}
	return cmdr.SuccessData(b)
}

// buildCLIGraph builds the CLI DI graph.
func buildCLIGraph(root *Sous, cli *CLI, g *graph.SousGraph, out, err io.Writer) *graph.SousGraph {
	g.Add(cli)
	g.Add(root)
	return g
}

// Invoke wraps the cmdr.CLI Invoke for logging.
func (cli *CLI) Invoke(args []string) cmdr.Result {
	start := time.Now()
	ls := cli.LogSink
	if ls == nil {
		ls = logging.NewLogSet(semv.Version{}, "sous", "", os.Stderr)
	}
	reportInvocation(ls, start, args)
	res := cli.CLI.Invoke(args)
	return res
}

func getLogSet(graph *graph.SousGraph) *logging.LogSet {
	type logSetScoop struct {
		*logging.LogSet
	}
	lss := &logSetScoop{}
	graph.MustInject(lss)
	return lss.LogSet
}

// NewSousCLI creates a new Sous cli app.
func NewSousCLI(di *graph.SousGraph, s *Sous, earlyLogging logging.LogSink, out, errout io.Writer) (*CLI, error) {

	stdout := cmdr.NewOutput(out)
	stderr := cmdr.NewOutput(errout)

	verbosity := &config.Verbosity{}

	cli := &CLI{
		LogSink: earlyLogging,
	}

	cli.CLI = &cmdr.CLI{
		Root: s,
		Out:  stdout,
		Err:  stderr,
		// HelpCommand is shown to the user if they type something that looks
		// like they want help, but which isn't recognised by Sous properly. It
		// uses the standard flag.ErrHelp value to decide whether or not to show
		// this.
		HelpCommand: os.Args[0] + " help",
		GlobalFlagSetFuncs: []func(*flag.FlagSet){
			AddVerbosityFlags(verbosity),
		},
	}

	cli.graph = buildCLIGraph(s, cli, di, out, errout)

	var addVerbosityOnce sync.Once

	cli.Hooks.Parsed = func(cmd cmdr.Command) error {
		addVerbosityOnce.Do(func() {
			cli.graph.Add(verbosity)
		})
		if registrant, ok := cmd.(Registrant); ok {
			registrant.RegisterOn(cli.graph)
		}
		cli.LogSink = getLogSet(cli.graph)
		return nil
	}

	// Before Execute is called on any command, inject its dependencies.
	cli.Hooks.PreExecute = func(cmd cmdr.Command) error {
		cli.graph.Hooks.NoValueForStructField = experiment.OptionalFieldHandler()
		return errors.Wrapf(cli.graph.Inject(cmd), "setup for execute")
	}

	cli.Hooks.PreFail = func(err error) cmdr.ErrorResult {
		if err != nil {
			originalErr := fmt.Sprint(err)
			err = errors.Cause(err)
			causeStr := err.Error()
			if originalErr != causeStr {
				messages.ReportLogFieldsMessage("Error occurred", logging.DebugLevel, logging.Log, originalErr)
			}
		}
		return EnsureErrorResult(err, "")
	}

	return cli, nil
}
