package cli

import (
	"flag"

	"github.com/opentable/sous/config"
	"github.com/opentable/sous/graph"
	"github.com/opentable/sous/lib"
	"github.com/opentable/sous/util/cmdr"
)

// SousInit is the command description for `sous init`
type SousInit struct {
	DeployFilterFlags config.DeployFilterFlags `inject:"optional"`
	OTPLFlags         config.OTPLFlags         `inject:"optional"`
	// DryRunFlag prints out the manifest but does not save it.
	DryRunFlag   bool `inject:"optional"`
	Target       graph.TargetManifest
	WD           graph.LocalWorkDirShell
	StateManager *graph.ClientStateManager
	User         sous.User
	flags        struct {
		Kind                 string
		SingularityRequestID string
	}
}

func init() { TopLevelCommands["init"] = &SousInit{} }

const sousInitHelp = `initialise a new sous project

usage: sous init

Sous init uses contextual information from your current source code tree and
repository to generate a basic configuration for that project. You will need to
flesh out some additional details.

init must be invoked in a git repository that has either an 'upstream' or
'origin' remote configured.

init will register the project on every known server.`

// Help returns the help string for this command
func (si *SousInit) Help() string { return sousInitHelp }

// RegisterOn adds flag sets for sous init to the dependency injector.
func (si *SousInit) RegisterOn(psy Addable) {
	// Add a zero DepoyFilterFlags to the graph, as we assume a clean build.
	psy.Add(&si.DeployFilterFlags)
	psy.Add(&si.OTPLFlags)
	psy.Add(graph.DryrunNeither)

	// ugh - there has to be a better way!
	si.OTPLFlags.Flavor = si.DeployFilterFlags.Flavor
}

// AddFlags adds the flags for sous init.
func (si *SousInit) AddFlags(fs *flag.FlagSet) {
	MustAddFlags(fs, &si.OTPLFlags, OtplFlagsHelp)
	fs.StringVar(&si.DeployFilterFlags.Flavor, "flavor", "", flavorFlagHelp)
	fs.StringVar(&si.DeployFilterFlags.Cluster, "cluster", "", clusterFlagHelp)
	fs.StringVar(&si.flags.Kind, "kind", "", kindFlagHelp)
	fs.StringVar(&si.flags.SingularityRequestID, "singularity-request-id", "", "Singularity request ID (must be used with -cluster)")
	fs.BoolVar(&si.DryRunFlag, "dryrun", false, "print out the created manifest but do not save it")
}

// Execute fulfills the cmdr.Executor interface.
func (si *SousInit) Execute(args []string) cmdr.Result {

	kind := sous.ManifestKind(si.flags.Kind)
	var skipHealth bool

	switch kind {
	default:
		return cmdr.UsageErrorf("kind not defined, pick one of %s or %s", sous.ManifestKindScheduled, sous.ManifestKindService)
	case sous.ManifestKindService:
		skipHealth = false
	case sous.ManifestKindScheduled, sous.ManifestKindOnDemand:
		skipHealth = true
	}

	if si.flags.SingularityRequestID != "" && si.DeployFilterFlags.Cluster == "" {
		return cmdr.UsageErrorf("If you specify -singularity-request-id you must also specify a single cluster using -cluster <cluster-name>")
	}

	m := si.Target.Manifest
	if skipHealth {
		for k, d := range m.Deployments {
			// Set the entire 'Startup' so it only has one non-zero field.
			d.Startup = sous.Startup{
				SkipCheck: true,
			}
			m.Deployments[k] = d
		}
	}

	cluster := si.DeployFilterFlags.Cluster

	state, err := si.StateManager.ReadState()
	if err != nil {
		return cmdr.InternalErrorf("getting current state: %s", err)
	}

	if _, ok := state.Defs.Clusters[cluster]; !ok && cluster != "" {
		return cmdr.UsageErrorf("cluster %q not defined, pick one of: %s", cluster, state.Defs.Clusters)
	}

	m.Kind = kind

	if cluster != "" {
		ds := sous.DeploySpecs{cluster: m.Deployments[cluster]}
		m.Deployments = ds
	}

	if si.DryRunFlag {
		return SuccessYAML(m)
	}

	if ok := state.Manifests.Add(m); !ok {
		return cmdr.UsageErrorf("manifest %q already exists", m.ID())
	}
	if err := si.StateManager.WriteState(state, si.User); err != nil {
		return EnsureErrorResult(err)
	}
	return SuccessYAML(m)
}
