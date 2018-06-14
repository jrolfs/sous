package sous

// has problems? go:generate ggen cmap.CMap(cmap.go) sous.Deployments(deployments.go) CMKey:DeployID Value:*Deployment

import (
	"fmt"
	"strings"

	"github.com/opentable/sous/util/logging"
)

type (
	// Deployment is a completely configured deployment of a piece of software.
	// It contains all the data necessary for Sous to create a single
	// deployment, which is a single version of a piece of software, running in
	// a single cluster.
	Deployment struct {
		// DeployConfig contains configuration info for this deployment,
		// including environment variables, resources, suggested instance count.
		DeployConfig `yaml:"inline"`
		// ClusterNickname is the human name for a cluster - it's taken from the
		// hash key that defines the cluster and is used in manifests to configure
		// cluster-local deployment config.
		ClusterName string
		// Cluster is the name of the cluster this deployment belongs to. Upon
		// parsing the Manifest, this will be set to the key in
		// Manifests.Deployments which points at this Deployment.
		Cluster *Cluster
		// SourceID is the precise version of the software to be deployed.
		SourceID SourceID
		// Flavor is the flavor of this deployment. See ManifestID for a fuller
		// description.
		Flavor string
		// Owners is a map of named owners of this repository. The type of this
		// field is subject to change.
		Owners OwnerSet
		// Kind is the kind of software that SourceRepo represents.
		Kind ManifestKind
		// User
		User User
	}
)

// Clone returns a deep copy of this deployment.
func (d Deployment) Clone() *Deployment {
	d.DeployConfig = d.DeployConfig.Clone()
	if d.Cluster != nil {
		d.Cluster = d.Cluster.Clone()
	}
	if d.Owners != nil {
		d.Owners = d.Owners.Clone()
	}
	return &d
}

// Clone returns a deep copy of this Volumes.
func (vs Volumes) Clone() Volumes {
	vols := make(Volumes, len(vs))
	copy(vols, vs)
	return vols
}

//EachField implements EachFielder
func (d Deployment) EachField(f logging.FieldReportFn) {
	sm := NewDeploymentSubmessage("", &d)
	sm.EachField(f)
}

func (d *Deployment) String() string {
	if d == nil {
		return "<nil>"
	}
	clusterName := "<unknown>"
	if d.Cluster != nil {
		clusterName = d.Cluster.Name
	}
	return fmt.Sprintf("%s %q @ %s %s", d.SourceID, d.Flavor, clusterName, d.DeployConfig.String())
}

// ID returns the DeployID of this deployment.
func (d *Deployment) ID() DeploymentID {
	return DeploymentID{
		ManifestID: d.ManifestID(),
		Cluster:    d.ClusterName,
	}
}

// Validate implements Flawed for State
func (d *Deployment) Validate() []Flaw {
	var flaws []Flaw

	if d.Kind == "" {
		flaws = append(flaws, NewFlaw(
			fmt.Sprintf("manifest %q missing Kind", d.ID()),
			func() error { d.Kind = ManifestKindService; return nil },
		))
	} else {
		flaws = append(flaws, d.Kind.Validate()...)
	}

	cf := d.DeployConfig.Validate()
	flaws = append(flaws, cf...)

	for _, f := range flaws {
		f.AddContext("deployment", d)
		f.AddContext("cluster", d.ClusterName)
	}

	return flaws
}

// ManifestID returns the ID of the Manifest describing this deployment.
func (d *Deployment) ManifestID() ManifestID {
	return ManifestID{
		Source: d.SourceID.Location,
		Flavor: d.Flavor,
	}
}

// DeploySpec returns a DeploySpec based on a Deployment
func (d *Deployment) DeploySpec() DeploySpec {
	return DeploySpec{
		DeployConfig: d.DeployConfig,
		Version:      d.SourceID.Version,
		clusterName:  d.ClusterName,
	}
}

// TabbedDeploymentHeaders returns the names of the fields for Tabbed, suitable
// for use with text/tabwriter.
func TabbedDeploymentHeaders() string {
	return "Cluster\t" +
		"Repo\t" +
		"Version\t" +
		"Offset\t" +
		"NumInstances\t" +
		"Owner\t" +
		"Resources\t" +
		"Env"
}

// Tabbed returns the fields of a deployment formatted in a tab delimited list.
func (d *Deployment) Tabbed() string {
	o := "<?>"
	for onr := range d.Owners {
		o = onr
		break
	}

	rs := []string{}
	for k, v := range d.DeployConfig.Resources {
		rs = append(rs, fmt.Sprintf("%s: %s", k, v))
	}
	es := []string{}
	for k, v := range d.DeployConfig.Env {
		es = append(es, fmt.Sprintf("%s: %s", k, v))
	}

	return fmt.Sprintf(
		"%s\t"+ //"Cluster\t" +
			"%s\t"+ //"Repo\t" +
			"%s\t"+ //"Version\t" +
			"%s\t"+ //"Offset\t" +
			"%d\t"+ //"NumInstances\t" +
			"%s\t"+ //"Owner\t" +
			"%s\t"+ //"Resources\t" +
			"%s", //"Env"
		d.ClusterName,
		d.SourceID.Location.Repo,
		d.SourceID.Version.String(),
		d.SourceID.Location.Dir,
		d.NumInstances,
		o,
		strings.Join(rs, ", "),
		strings.Join(es, ", "),
	)
}

// Name returns the DeployID.
func (d *Deployment) Name() DeploymentID {
	return d.ID()
}

// Equal returns true if two Deployments are equal.
func (d *Deployment) Equal(o *Deployment) bool {
	diff, _ := d.Diff(o)
	return !diff
}

// Differences records the differences between two Deployments
type Differences []string

func (diffs Differences) String() string {
	return strings.Join(diffs, "\n")
}

// Diff returns the differences between this deployment and another.
func (d *Deployment) Diff(o *Deployment) (bool, Differences) {
	if d.ID() != o.ID() {
		panic(fmt.Sprintf("attempt to compare deployment %q with %q", d.ID(), o.ID()))
	}
	var diffs Differences
	diff := func(format string, a ...interface{}) { diffs = append(diffs, fmt.Sprintf(format, a...)) }
	if d.ClusterName != o.ClusterName {
		diff("cluster name; this: %q; other: %q", d.ClusterName, o.ClusterName)
	}
	if !d.SourceID.Equal(o.SourceID) {
		diff("source id; this: %q; other: %q", d.SourceID, o.SourceID)
	}
	if d.Flavor != o.Flavor {
		diff("flavor; this: %q; other: %q", d.Flavor, o.Flavor)
	}
	if d.Kind != o.Kind {
		diff("kind; this: %q; other: %q", d.Kind, o.Kind)
	}

	// Schedule is only significant for Scheduled Jobs
	if d.Kind == ManifestKindScheduled {
		if d.Schedule != o.Schedule {
			diff("schedule; this: %q, other: %q", d.Schedule, o.Schedule)
		}
	}

	if len(d.Owners) != len(o.Owners) {
		diff("number of owners; this: %+v; other: %+v", len(d.Owners), len(o.Owners))
	}

	for owner := range d.Owners {
		if _, has := o.Owners[owner]; !has {
			diff("owner %s", owner)
		}
	}
	_, configDiffs := d.DeployConfig.Diff(o.DeployConfig)
	diffs = append(diffs, configDiffs...)

	return len(diffs) != 0, diffs
}
