package graph

import (
	sous "github.com/opentable/sous/lib"
	"github.com/opentable/sous/server"
	"github.com/samsalisbury/semv"
)

func newServerComponentLocator(
	ls LogSink, cfg LocalSousConfig, ins sous.Inserter, sm *ServerStateManager,
	rf *sous.ResolveFilter, ar *sous.AutoResolver, v semv.Version,
	qs sous.QueueSetFactory,
) server.ComponentLocator {
	cm := sous.MakeClusterManager(sm.StateManager)
	dm := sous.MakeDeploymentManager(sm.StateManager)
	return server.ComponentLocator{

		LogSink:           ls.LogSink,
		Config:            cfg.Config,
		Inserter:          ins,
		StateManager:      sm.StateManager,
		ClusterManager:    cm,
		DeploymentManager: dm,
		ResolveFilter:     rf,
		AutoResolver:      ar,
		Version:           v,
		QueueSetFactory:   qs,
	}

}

// NewR11nQueueSet returns a new queue set configured to start processing r11ns
// immediately.
func NewR11nQueueSet(d sous.Deployer, r sous.Registry) sous.QueueSetFactory {
	return func(rf *sous.ResolveFilter, sr sous.StateReader) sous.QueueSet {
		return sous.NewR11nQueueSet(sous.R11nQueueStartWithHandler(
			func(qr *sous.QueuedR11n) sous.DiffResolution {
				qr.Rectification.Begin(d, r, rf, sr)
				return qr.Rectification.Wait()
			}))
	}
}
