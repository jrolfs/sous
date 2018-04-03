package cli

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/opentable/sous/config"
	"github.com/opentable/sous/dto"
	"github.com/opentable/sous/graph"
	sous "github.com/opentable/sous/lib"
	"github.com/opentable/sous/server"
	"github.com/opentable/sous/util/cmdr"
	"github.com/opentable/sous/util/logging"
	"github.com/opentable/sous/util/logging/messages"
	"github.com/opentable/sous/util/restful"
	"github.com/samsalisbury/semv"
	"github.com/vbauerster/mpb"
	"github.com/vbauerster/mpb/decor"
	"golang.org/x/crypto/ssh/terminal"
)

// SousNewDeploy has the same interface as SousDeploy, but uses the new
// PUT /single-deployment endpoint to begin the deployment, and polls by
// watching the returned rectification URL.
type SousNewDeploy struct {
	DeployFilterFlags config.DeployFilterFlags `inject:"optional"`
	StateReader       graph.StateReader
	HTTPClient        *graph.ClusterSpecificHTTPClient
	TargetManifestID  graph.TargetManifestID
	LogSink           graph.LogSink
	dryrunOption      string
	waitStable        bool
	force             bool
	User              sous.User
	graph.LocalSousConfig
}

func init() { TopLevelCommands["newdeploy"] = &SousNewDeploy{} }

const sousNewDeployHelp = `deploys a new version into a particular cluster

usage: sous newdeploy -cluster <name> -tag <semver>

EXPERIMENTAL COMMAND: This may or may not yet do what it says on the tin.
Feel free to try it out, but if it breaks, you get to keep both pieces.

sous deploy will deploy the version tag for this application in the named
cluster.
`

// Help returns the help string for this command.
func (sd *SousNewDeploy) Help() string { return sousNewDeployHelp }

// AddFlags adds the flags for sous init.
func (sd *SousNewDeploy) AddFlags(fs *flag.FlagSet) {
	MustAddFlags(fs, &sd.DeployFilterFlags, DeployFilterFlagsHelp)

	fs.BoolVar(&sd.force, "force", false,
		"force deploy no matter if GDM already is at the correct version")
	fs.BoolVar(&sd.waitStable, "wait-stable", true,
		"wait for the deploy to complete before returning (otherwise, use --wait-stable=false)")
	fs.StringVar(&sd.dryrunOption, "dry-run", "none",
		"prevent rectify from actually changing things - "+
			"values are none,scheduler,registry,both")
}

// RegisterOn adds flag options to the graph.
func (sd *SousNewDeploy) RegisterOn(psy Addable) {
	psy.Add(graph.DryrunNeither)
	psy.Add(&sd.DeployFilterFlags)
}

// Execute creates the new deployment.
func (sd *SousNewDeploy) Execute(args []string) cmdr.Result {

	cluster := sd.DeployFilterFlags.Cluster

	newVersion, err := semv.Parse(sd.DeployFilterFlags.Tag)
	if err != nil {
		return cmdr.UsageErrorf("not semver: -tag %s", sd.DeployFilterFlags.Tag)
	}

	d := server.SingleDeploymentBody{}
	q := sd.TargetManifestID.QueryMap()
	q["cluster"] = cluster
	q["force"] = strconv.FormatBool(sd.force)

	updater, err := sd.HTTPClient.Retrieve("./single-deployment", q, &d, nil)
	if err != nil {
		return cmdr.InternalErrorf("Failed to retrieve current deployment: %s", err)
	}
	messages.ReportLogFieldsMessage("SousNewDeploy.Execute Retrieved Deployment",
		logging.ExtraDebug1Level, sd.LogSink, d)

	d.Deployment.Version = newVersion

	updateResponse, err := updater.Update(d, sd.User.HTTPHeaders())
	if err != nil {
		return cmdr.InternalErrorf("Failed to update deployment: %s", err)
	}

	if location := updateResponse.Location(); location != "" {
		fmt.Printf("Deployment queued: %s\n", location)
		client, err := restful.NewClient("", sd.LogSink, nil)
		if err != nil {
			return cmdr.InternalErrorf("Failed to create polling client: %s", err)
		}
		pollTime := sd.Config.PollIntervalForClient

		messages.ReportLogFieldsMessageToConsole("\n", logging.InformationLevel, sd.LogSink)

		var p *mpb.Progress
		var bar *mpb.Bar
		if terminal.IsTerminal(int(os.Stdin.Fd())) {
			p = mpb.New()
			// initialize bar with dynamic total and initial total guess = 80
			bar = p.AddBar(100,
				// indicate that total is dynamic
				mpb.BarDynamicTotal(),
				// trigger total auto increment by 1, when 18 % remains till bar completion
				mpb.BarAutoIncrTotal(18, 1),
				mpb.PrependDecorators(
					decor.CountersNoUnit("%d / %d", 12, 0),
				),
				mpb.AppendDecorators(
					decor.Percentage(5, 0),
				),
			)
		}

		result := PollDeployQueue(location, client, pollTime, bar, sd.LogSink)

		if terminal.IsTerminal(int(os.Stdin.Fd())) && bar != nil && p != nil {
			bar.SetTotal(100, true)
			bar.Incr(100)
			bar.Complete()
			p.Wait()
			p.RemoveBar(bar)
		}
		return result
	}
	return cmdr.Successf("Desired version for %q in cluster %q already %q",
		sd.TargetManifestID, cluster, sd.DeployFilterFlags.Tag)

}

func timeTrack(start time.Time) string {
	elapsed := time.Since(start)
	return elapsed.String()
}

// PollDeployQueue is used to poll server on status of Single Deployment.
func PollDeployQueue(location string, client restful.HTTPClient, pollAtempts int, bar *mpb.Bar, log logging.LogSink) cmdr.Result {
	start := time.Now()
	response := dto.R11nResponse{}
	location = "http://" + location

	for i := 0; i < pollAtempts; i++ {
		if bar != nil {
			bar.IncrBy(5)
		}
		if _, err := client.Retrieve(location, nil, &response, nil); err != nil {
			return cmdr.InternalErrorf("\n\tFailed to deploy: %s duration: %s\n", err, timeTrack(start))
		}

		queuePosition := response.QueuePosition

		if response.Resolution != nil && response.Resolution.Error != nil {
			return cmdr.InternalErrorf("\n\tFailed to deploy: %s duration: %s\n", response.Resolution.Error, timeTrack(start))
		}

		if queuePosition < 0 && response.Resolution != nil &&
			response.Resolution.DeployState != nil {

			if checkFinished(*response.Resolution) {
				if checkResolutionSuccess(*response.Resolution) {
					return cmdr.Successf("\n\tDeployment Complete %s, %s, duration: %s\n",
						response.Resolution.DeploymentID.String(), response.Resolution.DeployState.SourceID.Version, timeTrack(start))
				}
				//exit out to error handler
				return cmdr.InternalErrorf("Failed to deploy %s: %s", location, response.Resolution.Error)
			}

		}
		time.Sleep(1 * time.Second)
	}
	return cmdr.InternalErrorf("Failed to deploy %s after %d attempts for duration: %s\n", location, pollAtempts, timeTrack(start))
}

func checkFinished(resolution sous.DiffResolution) bool {
	switch resolution.Desc {
	default:
		return false
	case sous.CreateDiff, sous.ModifyDiff:
		return true
	}
}

/*
const (
	// DeployStatusAny represents any deployment status.
0	DeployStatusAny DeployStatus = iota
	// DeployStatusPending means the deployment has been requested in the
	// cluster, but is not yet running.
1	DeployStatusPending
	// DeployStatusActive means the deployment is up and running.
2	DeployStatusActive
	// DeployStatusFailed means the deployment has failed.
3	DeployStatusFailed
)
For now treating everything but Active as return failed, could look to changin in future
*/
func checkResolutionSuccess(resolution sous.DiffResolution) bool {
	//We know 3 is a failure and 2 is a success so far
	switch resolution.DeployState.Status {
	default:
		return false
	case sous.DeployStatusActive:
		return true
	}
}
