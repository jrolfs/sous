package storage

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opentable/sous/lib"
	"github.com/opentable/sous/util/yaml"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gitPrepare(t *testing.T, s *sous.State, remotepath, outpath string) {

	clobberDir(t, remotepath)
	clobberDir(t, outpath)

	runCmd(t, remotepath, "git", "init", "--template=/dev/null", "--bare")

	remoteAbs, err := filepath.Abs(remotepath)
	if err != nil {
		t.Fatal(err)
	}

	runCmd(t, outpath, "git", "init", "--template=/dev/null")
	runCmd(t, outpath, "git", "config", "user.email", "sous-test@testing.example.com")
	runCmd(t, outpath, "git", "config", "user.name", "sous-test@testing.example.com")
	runCmd(t, outpath, "git", "remote", "add", "origin", "file://"+remoteAbs)

	dsm := NewDiskStateManager(outpath)
	dsm.WriteState(s, testUser)

	runCmd(t, outpath, "git", "add", ".")
	runCmd(t, outpath, "git", "commit", "--no-gpg-sign", "-a", "-m", "birthday")
	runCmd(t, outpath, "git", "push", "-u", "origin", "master")
}

func clobberDir(t *testing.T, path string) {
	if err := os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(path, os.ModePerm); err != nil {
		t.Fatal(err)
	}
}

func runCmd(t *testing.T, path string, cmd ...string) {
	gitCmd := exec.Command(cmd[0], cmd[1:]...)
	gitCmd.Dir = path
	out, err := gitCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%q errored: %v\n %s", strings.Join(cmd, " "), err, out)
	}
}

var testUser = sous.User{Name: "Test User", Email: "test@user.com"}

func TestGitStateManager_WriteState_success(t *testing.T) {
	require := require.New(t)

	s := exampleState()

	clobberDir(t, "testdata/result")
	gitPrepare(t, s, "testdata/remote", "testdata/out")

	gsm := NewGitStateManager(NewDiskStateManager("testdata/out"))

	require.NoError(gsm.WriteState(s, testUser))

	// eh? hacky, but we actually only care about Sous files

	remoteAbs, err := filepath.Abs("testdata/remote")
	if err != nil {
		t.Fatal(err)
	}
	runCmd(t, "testdata", "git", "clone", "file://"+remoteAbs, "result")

	os.RemoveAll("testdata/result/.git")

	d := exec.Command("diff", "-r", "testdata/in", "testdata/result")

	if out, err := d.CombinedOutput(); err != nil {
		t.Fatalf("Output not as expected: %s;\n%s", err, string(out))
	}
}

func TestGitStateManager_WriteState_multiple_manifests(t *testing.T) {

	s := exampleState()

	gitPrepare(t, s, "testdata/remote", "testdata/out")

	gsm := NewGitStateManager(NewDiskStateManager("testdata/out"))

	// Modify one of the manifests.
	m, ok := s.Manifests.Any(func(m *sous.Manifest) bool { return m.Source.Repo == "github.com/opentable/sous" })
	if !ok {
		t.Fatalf("no manifests found")
	}
	m.Deployments["cluster-1"].Env["NEWVAR"] = "YOLO"

	// Modify the other manifest.
	m, ok = s.Manifests.Any(func(m *sous.Manifest) bool { return m.Source.Repo == "github.com/user/project" })
	if !ok {
		t.Fatalf("no manifests found")
	}
	m.Deployments["other-cluster"].Env["NEWVAR"] = "YOLO"

	s.Manifests.Set(m.ID(), m)

	// 4. Attempt to write new manifest, expect error.
	actualErr := gsm.WriteState(s, testUser)
	if actualErr == nil {
		t.Fatal("erroneously allowed writing state with modifications in multiple files")
	}
}

func TestGitReadState(t *testing.T) {
	require := require.New(t)

	gsm := NewGitStateManager(NewDiskStateManager("testdata/in"))

	actual, err := gsm.ReadState()
	require.NoError(err)

	expected := exampleState()

	sameYAML(t, actual, expected)
}

func sameYAML(t *testing.T, actual *sous.State, expected *sous.State) {
	assert := assert.New(t)
	require := require.New(t)

	actualManifests := actual.Manifests.Snapshot()
	expectedManifests := expected.Manifests.Snapshot()
	assert.Len(actualManifests, len(expectedManifests))
	for mid, manifest := range expectedManifests {
		actual := *actualManifests[mid]
		assert.Contains(actualManifests, mid)
		if !assert.Equal(actual, *manifest) {
			_, differences := actual.Diff(manifest)
			t.Logf("DIFFERENCES (%q): %#v", mid, differences)
		}
	}

	actualYAML, err := yaml.Marshal(actual)
	require.NoError(err)
	expectedYAML, err := yaml.Marshal(expected)
	require.NoError(err)
	assert.Equal(actualYAML, expectedYAML)
}

func runScript(t *testing.T, script string, dir ...string) {
	lines := strings.Split(script, "\n")
	for _, l := range lines {
		words := strings.Split(strings.Trim(l, " \t"), " ")
		cmd := exec.Command(words[0], words[1:]...)
		if len(dir) > 0 {
			cmd.Dir = dir[0]
		}
		cmd.Env = []string{"GIT_CONFIG_NOSYSTEM=true", "HOME=none", "XDG_CONFIG_HOME=none"}
		//log.Print(cmd)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatal("x", err, cmd, string(out))
		}
	}
}

// setupManagers creates a local clone of a remote at testdata/remote.
// It returns a GitStateManager rooted in the clone, and a DiskStateManager
// rooted in the remote.
func setupManagers(t *testing.T) (clone *GitStateManager, remote *DiskStateManager) {
	// Setup testdata/origin as the remote.
	runScript(t, `rm -rf testdata/origin testdata/target
	cp -a testdata/in testdata/origin`)
	runScript(t, `git init
	git add .
	git config --local receive.denyCurrentBranch ignore
	git config user.email sous@opentable.com
	git config user.name Sous
	git config user.signingkey ""
	git commit -m ""`, `testdata/origin`)
	runScript(t, `git clone origin target`, `testdata`)
	runScript(t, `git config user.email sous@opentable.com
	git config user.name Sous`, `testdata/target`)

	gsm := NewGitStateManager(NewDiskStateManager("testdata/target"))
	dsm := NewDiskStateManager(`testdata/origin`)

	return gsm, dsm
}

func TestGitPulls(t *testing.T) {
	require := require.New(t)
	gsm, remote := setupManagers(t)

	actual, err := gsm.ReadState()
	require.NoError(err)

	expected := exampleState()
	sameYAML(t, actual, expected)

	expected.Manifests.Add(&sous.Manifest{Source: sous.SourceLocation{Repo: "github.com/opentable/brandnew"}})
	remote.WriteState(expected, sous.User{})
	expected, err = remote.ReadState()
	require.NoError(err)
	runScript(t, `git add .
	git commit -m ""`, `testdata/origin`)

	actual, err = gsm.ReadState()
	require.NoError(err)

	sameYAML(t, actual, expected)
}

func TestGitPushes(t *testing.T) {
	require := require.New(t)
	gsm, remote := setupManagers(t)

	expected, err := gsm.ReadState()
	require.NoError(err)

	expected.Manifests.Add(&sous.Manifest{Source: sous.SourceLocation{Repo: "github.com/opentable/brandnew"}})
	require.NoError(gsm.WriteState(expected, testUser))
	expected, err = gsm.ReadState()
	require.NoError(err)

	runScript(t, `git reset --hard`, `testdata/origin`) //in order to reflect the change
	actual, err := remote.ReadState()
	require.NoError(err)
	sameYAML(t, actual, expected)
}

func TestGitConflicts(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	gsm, remote := setupManagers(t)

	actual, err := gsm.ReadState()
	require.NoError(err)

	expected := exampleState()

	expected.Manifests.Add(&sous.Manifest{Source: sous.SourceLocation{Repo: "github.com/opentable/brandnew"}})
	remote.WriteState(expected, sous.User{})

	expected, err = remote.ReadState()
	require.NoError(err)
	runScript(t, `git add .
	git commit -m ""`, `testdata/origin`)

	actual.Manifests.Add(&sous.Manifest{Source: sous.SourceLocation{Repo: "github.com/opentable/newhotness"}})

	actualErr := gsm.WriteState(actual, testUser)
	assert.NoError(actualErr)

	actual, err = gsm.ReadState()
	require.NoError(err)

	// Add the thing we wrote to actual to expected as well, since actual now
	// contains the two sets of changes merged.
	expected.Manifests.Add(&sous.Manifest{
		Source: sous.SourceLocation{Repo: "github.com/opentable/newhotness"},
		// Kind, Owners, Deployments have to be set to non-nil because
		// when they are read, the flaws library replaces nils with non-nils
		// for these fields.
		Kind:        sous.ManifestKindService,
		Owners:      []string{},
		Deployments: sous.DeploySpecs{},
	})

	sameYAML(t, actual, expected)
}

func TestGitRemoteDelete(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	gsm, remote := setupManagers(t)

	_, err := gsm.ReadState()
	require.NoError(err)

	expected := exampleState()

	expected.Manifests.Add(&sous.Manifest{Source: sous.SourceLocation{Repo: "github.com/opentable/brandnew"}})
	remote.WriteState(expected, sous.User{})

	_, err = remote.ReadState()
	require.NoError(err)
	runScript(t, `git add .
	git commit -m ""`, `testdata/origin`)

	actual, err := gsm.ReadState()
	require.NoError(err)

	runScript(t, `rm -rf manifests/github.com/opentable/brandnew.yaml
	git commit -am ""`, `testdata/origin`)
	require.NoError(err)

	actual.Manifests.Add(&sous.Manifest{Source: sous.SourceLocation{Repo: "github.com/opentable/newhotness"}})
	actualErr := gsm.WriteState(actual, testUser)
	assert.NoError(actualErr)

	expected, err = remote.ReadState()
	require.NoError(err)

	actual, err = gsm.ReadState()
	require.NoError(err)

	// Add the thing we wrote to actual to expected as well, since actual now
	// contains the two sets of changes merged.
	expected.Manifests.Add(&sous.Manifest{
		Source: sous.SourceLocation{Repo: "github.com/opentable/newhotness"},
		// Kind, Owners, Deployments have to be set to non-nil because
		// when they are read, the flaws library replaces nils with non-nils
		// for these fields.
		Kind:        sous.ManifestKindService,
		Owners:      []string{},
		Deployments: sous.DeploySpecs{},
	})

	sameYAML(t, actual, expected)
}

func TestGitReadState_empty(t *testing.T) {
	gsm := NewGitStateManager(NewDiskStateManager("testdata/nonexistent"))
	actual, err := gsm.ReadState()
	if err != nil && !os.IsNotExist(errors.Cause(err)) {
		t.Fatal(err)
	}
	d, err := actual.Deployments()
	if err != nil {
		t.Fatal(err)
	}
	if d.Len() != 0 {
		t.Errorf("got len %d; want %d", d.Len(), 0)
	}
}
