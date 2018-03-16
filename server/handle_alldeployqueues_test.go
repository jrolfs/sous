package server

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	sous "github.com/opentable/sous/lib"
	"github.com/pborman/uuid"
)

func testQueueSetFactory(qs sous.QueueSet) sous.QueueSetFactory {
	return func(*sous.ResolveFilter, sous.StateReader) sous.QueueSet {
		return qs
	}
}

// TestNewAllDeployQueuesResource checks that the same queue set passed to the
// constructor makes its way to the get handler.
func TestNewAllDeployQueuesResource(t *testing.T) {
	qs := &sous.R11nQueueSet{}
	qsf := testQueueSetFactory(qs)
	c := ComponentLocator{
		QueueSetFactory: qsf,
	}
	adq := newAllDeployQueuesResource(c)
	rm := routemap(c)

	got := adq.Get(rm, nil, &http.Request{URL: &url.URL{}}, nil).(*GETAllDeployQueuesHandler)
	if got.QueueSet != qs {
		t.Errorf("got different queueset")
	}
}

func TestGETAllDeployQueuesHandler_Exchange(t *testing.T) {
	t.Run("empty queue", func(t *testing.T) {
		data, status := setupAllQueueSetsExchange(t)
		assertStatusCode200(t, status)
		dqr := assertIsDeploymentQueuesResponse(t, data)
		assertQueueLength(t, dqr, sous.DeploymentID{}, 0)
		assertNumQueues(t, dqr, 0)
	})

	t.Run("one DeploymentID", func(t *testing.T) {
		data, status := setupAllQueueSetsExchange(t, newDid("one"))
		assertStatusCode200(t, status)
		dqr := assertIsDeploymentQueuesResponse(t, data)
		assertQueueLength(t, dqr, sous.DeploymentID{}, 0)
		assertNumQueues(t, dqr, 1)
		assertQueueLength(t, dqr, newDid("one"), 1)
	})

	t.Run("two unique DeploymentIDs", func(t *testing.T) {
		data, status := setupAllQueueSetsExchange(t, newDid("one"), newDid("two"))
		assertStatusCode200(t, status)
		dqr := assertIsDeploymentQueuesResponse(t, data)
		assertQueueLength(t, dqr, sous.DeploymentID{}, 0)
		assertNumQueues(t, dqr, 2)
		assertQueueLength(t, dqr, newDid("one"), 1)
		assertQueueLength(t, dqr, newDid("two"), 1)
	})

	t.Run("same deployment twice", func(t *testing.T) {
		data, status := setupAllQueueSetsExchange(t, newDid("one"), newDid("one"))
		assertStatusCode200(t, status)
		dqr := assertIsDeploymentQueuesResponse(t, data)
		assertQueueLength(t, dqr, sous.DeploymentID{}, 0)
		assertNumQueues(t, dqr, 1)
		assertQueueLength(t, dqr, newDid("one"), 2)
	})

}

// setupAllQueueSetsExchange generates a R11nQueueSet having a new R11n pushed
// to it for each id provided in dids. It injects this into a new
// GETAllDeployQueuesHandler and calls Exchange, returning the result.
func setupAllQueueSetsExchange(t *testing.T, dids ...sous.DeploymentID) (interface{}, int) {
	t.Helper()
	qs := sous.NewR11nQueueSet()
	for _, did := range dids {
		r11n := &sous.Rectification{
			Pair: sous.DeployablePair{},
		}
		r11n.Pair.SetID(did)
		if _, ok := qs.Push(r11n); !ok {
			t.Fatal("precondition failed: failed to push r11n")
		}
	}
	handler := &GETAllDeployQueuesHandler{
		QueueSet: qs,
	}
	return handler.Exchange()
}

func assertStatusCode200(t *testing.T, gotStatus int) {
	t.Helper()
	const wantStatusCode = 200
	if gotStatus != wantStatusCode {
		t.Errorf("got %d; want %d", gotStatus, wantStatusCode)
	}
}

func assertIsDeploymentQueuesResponse(t *testing.T, data interface{}) DeploymentQueuesResponse {
	t.Helper()
	dr, ok := data.(DeploymentQueuesResponse)
	if !ok {
		t.Fatalf("got a %T; want a %T", data, dr)
		return DeploymentQueuesResponse{}
	}
	return dr
}

func assertNumQueues(t *testing.T, dr DeploymentQueuesResponse, wantLen int) {
	t.Helper()
	gotLen := len(dr.Queues)
	if gotLen != wantLen {
		t.Fatalf("got %d queued deployments; want %d", gotLen, wantLen)
	}
}

func assertQueueLength(t *testing.T, dr DeploymentQueuesResponse, did sous.DeploymentID, wantCount int) {
	t.Helper()
	gotCount := dr.Queues[did.String()].Length
	if gotCount != wantCount {
		t.Errorf("got %d queued rectifications for %q; want %d",
			gotCount, did, wantCount)
	}
}

func newDid(repo string) sous.DeploymentID {
	return sous.DeploymentID{
		ManifestID: sous.ManifestID{
			Source: sous.SourceLocation{
				Repo: repo,
			},
		},
	}
}

// TestGETDeploymentsHandler_Exchange_async should be run with the -race flag.
func TestGETAllDeployQueuesHandler_Exchange_async(t *testing.T) {

	// Start writing to a new queueset that's also being processed rapidly.
	qh := func(*sous.QueuedR11n) (dr sous.DiffResolution) { return }
	queues := sous.NewR11nQueueSet(sous.R11nQueueStartWithHandler(qh))
	go func() {
		for {
			did := newDid(uuid.New())
			did.Cluster = uuid.New()
			r11n := newR11n("")
			r11n.Pair.SetID(did)
			queues.Push(r11n)
			time.Sleep(time.Millisecond)
		}
	}()

	// Set up a handler using the above busy queue set.
	dh := GETAllDeployQueuesHandler{QueueSet: queues}

	// Start calling Exchange in a hot loop.
	for i := 0; i < 1000; i++ {
		dh.Exchange()
	}
}
