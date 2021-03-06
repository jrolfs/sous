package actions

import (
	"fmt"
	"io"
	"time"

	sous "github.com/opentable/sous/lib"
	"github.com/opentable/sous/util/logging"
	"github.com/pkg/errors"
)

type (
	updateMessage struct {
		callerInfo logging.CallerInfo
		tries      int
		sid        sous.SourceID
		did        sous.DeploymentID
		manifest   *sous.Manifest
		user       sous.User
		interval   logging.MessageInterval
		err        error
	}
)

func newUpdateBeginMessage(tries int,
	sid sous.SourceID,
	did sous.DeploymentID,
	user sous.User,
	start time.Time) updateMessage {
	return updateMessage{
		callerInfo: logging.GetCallerInfo(logging.NotHere()),
		tries:      tries,
		sid:        sid,
		did:        did,
		user:       user,
		interval:   logging.OpenInterval(start),
	}
}

func newUpdateSuccessMessage(tries int,
	sid sous.SourceID,
	did sous.DeploymentID,
	manifest *sous.Manifest,
	user sous.User,
	start time.Time) updateMessage {
	return updateMessage{
		callerInfo: logging.GetCallerInfo(logging.NotHere()),
		tries:      tries,
		sid:        sid,
		did:        did,
		manifest:   manifest,
		user:       user,
		interval:   logging.CompleteInterval(start),
	}
}

func newUpdateErrorMessage(tries int,
	sid sous.SourceID,
	did sous.DeploymentID,
	user sous.User,
	start time.Time,
	err error) updateMessage {
	return updateMessage{
		callerInfo: logging.GetCallerInfo(logging.NotHere()),
		tries:      tries,
		sid:        sid,
		did:        did,
		user:       user,
		interval:   logging.OpenInterval(start),
		err:        err,
	}
}

func (msg updateMessage) Message() string {
	if msg.err != nil {
		return "Error during update"
	}
	if msg.interval.Complete() {
		return "Update successful"
	}
	return "Beginning update"
}

func (msg updateMessage) EachField(fn logging.FieldReportFn) {
	fn("@loglov3-otl", logging.SousUpdateV1)
	msg.callerInfo.EachField(fn)
	msg.interval.EachField(fn)

	fn("try-number", msg.tries)
	fn("source-id", msg.sid.String())
	fn("deploy-id", msg.did.String())
	fn("user-email", msg.user.Email)

	if msg.err != nil {
		fn("error", msg.err.Error())
	}
}

func (msg updateMessage) WriteToConsole(console io.Writer) {
	if msg.err != nil {
		if _, err := console.Write([]byte(msg.err.Error())); err != nil {
			fmt.Println(errors.Wrap(err, msg.err.Error()))
		}
		return
	}
	if msg.interval.Complete() {
		version := msg.sid.Version.String()
		numInstances := msg.manifest.Deployments[msg.did.Cluster].NumInstances

		if _, err := fmt.Fprintf(console, "Updated global manifest: %d instances of version %s\n",
			numInstances, version); err != nil {
			fmt.Println(errors.Wrap(err, msg.err.Error()))
		}
	}
}
