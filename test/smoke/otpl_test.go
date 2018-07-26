//+build smoke

package smoke

import (
	"fmt"
	"strings"
	"testing"

	"github.com/opentable/sous/util/filemap"
)

func dockerBuildAddArtifactInit(t *testing.T, f *testFixture, client *sousClient, flags *sousFlags, transforms ...ManifestTransform) (dockerRef string) {
	t.Helper()

	dockerRef = dockerBuildAddArtifact(t, f, client, flags)

	client.MustRun(t, "init", flags.SousInitFlags(), "-use-otpl-deploy")
	client.TransformManifest(t, flags, transforms...)

	return dockerRef
}

func dockerBuildAddArtifact(t *testing.T, f *testFixture, client *sousClient, flags *sousFlags) (dockerRef string) {
	t.Helper()
	tag := flags.tag
	if tag == "" {
		t.Fatalf("you must add a non-empty tag flag")
	}
	reg := client.Config.Docker.RegistryHost
	repo := flags.repo
	dockerTag := f.IsolatedVersionTag(t, tag)
	dockerRepo := fmt.Sprintf("%s/%s", reg, repo)
	dockerRef = fmt.Sprintf("%s:%s", dockerRepo, dockerTag)

	mustDoCMD(t, client.Dir, "docker", "build", "-t", dockerRef, ".")
	mustDoCMD(t, client.Dir, "docker", "push", dockerRef)

	client.MustRun(t, "artifact add", nil, "-docker-image", dockerRepo, "-repo", repo, "-tag", tag)

	return dockerRef
}

func TestOTPL(t *testing.T) {

	// FixedDimension is because otpl deploy can only work with simple dockerfile
	// projects, not split build projects.
	pf := pfs.newParallelTestFixture(t, Matrix().FixedDimension("project", "simple"))

	pf.RunMatrix(

		PTest{Name: "artifact-add", Test: func(t *testing.T, f *testFixture) {
			client := f.setupProject(t, f.Projects.HTTPServer())

			flags := &sousFlags{tag: "1.2.3", repo: "github.com/some-user/project1"}

			dockerRef := dockerBuildAddArtifact(t, f, client, flags)

			output := client.MustRun(t, "artifact get", flags)

			if !strings.Contains(output, dockerRef) {
				// TODO SS: Figure out how to do this assertion given that we do
				// not store the Docker tag sent, only the  digest.
				//t.Errorf("output did not contain %q; was:\n%s", dockerRef, output)
			} else {
				// TODO SS: Remove next line once we have the assertion above.
				t.Logf(output)
			}
		}},

		PTest{Name: "build-init-deploy", Test: func(t *testing.T, f *testFixture) {

			client := f.setupProject(t, f.Projects.HTTPServer().Merge(filemap.FileMap{
				"config/cluster1/singularity.json": `
				{
					"requestId": "request1",
					"resources": {
						"cpus": 0.01,
						"memoryMb": 1,
						"numPorts": 3
					}
				}`,
				"config/cluster1/singularity-request.json": `
				{
					"id": "request1",
					"requestType": "SERVICE",
					"owners": [
					    "test-user1@example.com"
					],
					"instances": 3
				}`,
			}))

			flags := &sousFlags{
				kind:    "http-service",
				repo:    "github.com/build-init-deploy-user/project1",
				tag:     "1.2.3",
				cluster: "cluster1",
			}

			dockerBuildAddArtifactInit(t, f, client, flags, setMinimalMemAndCPUNumInst1)

			client.MustRun(t, "deploy", flags.SousDeployFlags())

			reqID := f.DefaultSingReqID(t, flags)
			assertActiveStatus(t, f, reqID)
			assertSingularityRequestTypeService(t, f, reqID)
			assertNonNilHealthCheckOnLatestDeploy(t, f, reqID)
		}},

		PTest{Name: "fail-unknown-fields", Test: func(t *testing.T, f *testFixture) {

			client := f.setupProject(t, f.Projects.HTTPServer().Merge(filemap.FileMap{
				"config/cluster1/singularity.json": `
				{
					"requestId": "request1",
					"resources": {
						"cpus": 0.01,
						"memoryMb": 1,
						"numPorts": 3
					}
				}`,
				"config/cluster1/singularity-request.json": `
				{
					id: "request1",
					"requestType": "WORKER",
					"owners": [
					    "test-user1@example.com"
					],
					"slavePlacement": "SEPARATE_BY_REQUEST",
					"instances": 3,
					"rackSensitive": false,
					"loadBalanced": false
				}`,
			}))

			flags := &sousFlags{
				kind:    "http-service",
				repo:    "github.com/build-init-deploy-user/project1",
				tag:     "1.2.3",
				cluster: "cluster1",
			}

			client.MustFail(t, "init", flags.SousInitFlags(), "-use-otpl-deploy")
		}},
	)
}
