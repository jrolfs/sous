package actions

import (
	"bytes"
	"os"
	"testing"

	sous "github.com/opentable/sous/lib"
	"github.com/opentable/sous/util/logging"
	"github.com/opentable/sous/util/restful"
	"github.com/opentable/sous/util/restful/restfultest"
	"github.com/opentable/sous/util/yaml"
	"github.com/samsalisbury/semv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetBadKeys_badkey(t *testing.T) {
	mani := sous.ManifestFixture("simple")
	env := make(map[string]string)
	env["secrets"] = "foo"

	deploy := mani.Deployments["ci"]
	deploy.Env = env
	mani.Deployments["ci"] = deploy

	bad := getBadKeys(*mani, []string{"secret", "password"})
	assert.True(t, len(bad) > 0)
}

func TestGetBadKeys_goodkeys(t *testing.T) {
	mani := sous.ManifestFixture("simple")
	env := make(map[string]string)
	env["bar"] = "foo"

	deploy := mani.Deployments["ci"]
	deploy.Env = env
	mani.Deployments["ci"] = deploy

	bad := getBadKeys(*mani, []string{"secret", "password"})
	assert.True(t, len(bad) == 0)
}

func TestManifestSet_ErrorOnSourceLocation(t *testing.T) {
	project1 := sous.SourceLocation{Repo: "github.com/user/randomprojectnotmatching"}

	mid := sous.ManifestID{
		Source: sous.SourceLocation{
			Repo: project1.Repo,
		},
	}

	//this will not match the source location, therefore should error on update
	mani := sous.ManifestFixture("simple")

	mani.Flavor = "vanilla"
	yml, err := yaml.Marshal(mani)
	require.NoError(t, err)
	in := bytes.NewBuffer(yml)

	updater, upctl := restfultest.NewUpdateSpy()

	upctl.Any(
		"Update",
		nil,
	)

	up := updater.(restful.Updater)

	sms := &ManifestSet{
		ManifestID: mid,

		InReader: in,
		LogSink:  logging.NewLogSet(semv.MustParse("0.0.0"), "", "", os.Stderr),
		Updater:  &up,
	}

	assert.Error(t, sms.Do(), "this should error since source is different")
}

func TestManifestSet(t *testing.T) {
	mani := sous.ManifestFixture("simple")
	mani.Flavor = "vanilla"

	mid := mani.ID()

	yml, err := yaml.Marshal(mani)
	require.NoError(t, err)
	in := bytes.NewBuffer(yml)

	updater, upctl := restfultest.NewUpdateSpy()

	upctl.Any(
		"Update",
		nil,
	)

	up := updater.(restful.Updater)

	sms := &ManifestSet{
		ManifestID: mid,

		InReader: in,
		LogSink:  logging.NewLogSet(semv.MustParse("0.0.0"), "", "", os.Stderr),
		Updater:  &up,
	}

	err = sms.Do()
	assert.NoError(t, err)

	if assert.Len(t, upctl.Calls(), 1) {
		args := upctl.Calls()[0].PassedArgs()
		assert.Equal(t, args.Get(0).(*sous.Manifest).Flavor, "vanilla")
	}
}
