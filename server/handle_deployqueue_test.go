package server

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"

	sous "github.com/opentable/sous/lib"
)

func makeRequestWithQuery(t *testing.T, query string) *http.Request {
	t.Helper()
	u, err := url.Parse("?" + query)
	if err != nil {
		t.Fatalf("setup failed: parsing url: %s", err)
	}
	return &http.Request{URL: u}
}

// TestNewDeployQueueResource checks that the same queue set passed to the
// constructor makes its way to the get handler.
func TestNewDeployQueueResource(t *testing.T) {
	qs := &sous.R11nQueueSet{}
	c := ComponentLocator{
		QueueSetFactory: testQueueSetFactory(qs),
	}
	rm := routemap(c)
	dq := newDeployQueueResource(c)

	got := dq.Get(rm, nil, &http.Request{URL: &url.URL{}}, nil).(*GETDeployQueueHandler)
	if got.QueueSet != qs {
		t.Errorf("got different queueset")
	}
}

func TestDeployQueueResource_Get_no_errors(t *testing.T) {

	testCases := []struct {
		desc    string
		query   string
		wantDID sous.DeploymentID
	}{
		{
			desc:  "cluster,flavor,offset,repo",
			query: "cluster=cluster1&flavor=flavor1&offset=dir1&repo=github.com%2Fuser1%2Frepo1",
			wantDID: sous.DeploymentID{
				ManifestID: sous.ManifestID{
					Source: sous.SourceLocation{
						Repo: "github.com/user1/repo1",
						Dir:  "dir1",
					},
					Flavor: "flavor1",
				},
				Cluster: "cluster1",
			},
		},
		{
			desc:  "cluster,offset,repo",
			query: "cluster=cluster1&offset=dir1&repo=github.com%2Fuser1%2Frepo1",
			wantDID: sous.DeploymentID{
				ManifestID: sous.ManifestID{
					Source: sous.SourceLocation{
						Repo: "github.com/user1/repo1",
						Dir:  "dir1",
					},
				},
				Cluster: "cluster1",
			},
		},
		{
			desc:  "cluster,flavor,repo",
			query: "cluster=cluster1&flavor=flavor1&repo=github.com%2Fuser1%2Frepo1",
			wantDID: sous.DeploymentID{
				ManifestID: sous.ManifestID{
					Source: sous.SourceLocation{
						Repo: "github.com/user1/repo1",
					},
					Flavor: "flavor1",
				},
				Cluster: "cluster1",
			},
		},
		{
			desc:  "cluster,repo",
			query: "cluster=cluster1&repo=github.com%2Fuser1%2Frepo1",
			wantDID: sous.DeploymentID{
				ManifestID: sous.ManifestID{
					Source: sous.SourceLocation{
						Repo: "github.com/user1/repo1",
					},
				},
				Cluster: "cluster1",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			qs := &sous.R11nQueueSet{}
			c := ComponentLocator{QueueSetFactory: testQueueSetFactory(qs)}
			rm := routemap(c)

			dr := &DeployQueueResource{context: c}
			req := makeRequestWithQuery(t, tc.query)
			got := dr.Get(rm, nil, req, nil).(*GETDeployQueueHandler)

			gotDID := got.DeploymentID
			if gotDID != tc.wantDID {
				t.Errorf("got DeploymentID:\n%#v; want:\n%#v", gotDID, tc.wantDID)
			}

			if got.DeploymentIDErr != nil {
				t.Errorf("unexpected error: %s", got.DeploymentIDErr)
			}
		})
	}
}

func TestDeployQueueResource_Get_DeploymentID_errors(t *testing.T) {

	testCases := []struct {
		query      string
		wantDIDErr string
	}{
		{
			query:      "repo=github.com%2Fuser1%2Frepo1",
			wantDIDErr: `No cluster given`,
		},
		{
			query:      "cluster=cluster1",
			wantDIDErr: `No repo given`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.wantDIDErr, func(t *testing.T) {
			qs := &sous.R11nQueueSet{}
			c := ComponentLocator{QueueSetFactory: testQueueSetFactory(qs)}
			rm := routemap(c)
			dr := &DeployQueueResource{context: c}
			req := makeRequestWithQuery(t, tc.query)
			got := dr.Get(rm, nil, req, nil).(*GETDeployQueueHandler)

			gotDIDErr := got.DeploymentIDErr
			if gotDIDErr == nil || gotDIDErr.Error() != tc.wantDIDErr {
				t.Fatalf("got error: %v; want %q", gotDIDErr, tc.wantDIDErr)
			}
		})
	}

}

func TestGETDeployQueueHandler_Exchange(t *testing.T) {

	queues := sous.NewR11nQueueSet()
	queuedOne1, ok := queues.Push(newR11n("one"))
	if !ok {
		t.Fatal("setup failed to push r11n")
	}
	queuedTwo1, ok := queues.Push(newR11n("two"))
	if !ok {
		t.Fatal("setup failed to push r11n")
	}
	queuedTwo2, ok := queues.Push(newR11n("two"))
	if !ok {
		t.Fatal("setup failed to push r11n")
	}

	t.Run("nonexistent_deployID", func(t *testing.T) {
		gdh := &GETDeployQueueHandler{
			QueueSet:     queues,
			DeploymentID: newDid("nonexistent"),
		}
		body, gotStatus := gdh.Exchange()
		gotResponse := body.(deployQueueResponse)
		const wantStatus = 404
		if gotStatus != wantStatus {
			t.Errorf("got status %d; want %d", gotStatus, wantStatus)
		}
		gotLen := len(gotResponse.Queue)
		wantLen := 0
		if gotLen != wantLen {
			t.Errorf("got %d queued; want %d", gotLen, wantLen)
		}
	})
	t.Run("one_queued", func(t *testing.T) {
		gdh := &GETDeployQueueHandler{
			QueueSet:     queues,
			DeploymentID: newDid("one"),
		}
		body, gotStatus := gdh.Exchange()
		gotResponse := body.(deployQueueResponse)
		const wantStatus = 200
		if gotStatus != wantStatus {
			t.Errorf("got status %d; want %d", gotStatus, wantStatus)
		}
		gotLen := len(gotResponse.Queue)
		wantLen := 1
		if gotLen != wantLen {
			t.Errorf("got %d queued; want %d", gotLen, wantLen)
		}
		item := gotResponse.Queue[0]
		wantR11nID := queuedOne1.ID
		gotR11nID := item.ID
		if gotR11nID != wantR11nID {
			t.Errorf("got R11nID %q; want %q", gotR11nID, wantR11nID)
		}

	})
	t.Run("two_queued", func(t *testing.T) {
		gdh := &GETDeployQueueHandler{
			QueueSet:     queues,
			DeploymentID: newDid("two"),
		}
		body, gotStatus := gdh.Exchange()
		gotResponse := body.(deployQueueResponse)
		const wantStatus = 200
		if gotStatus != wantStatus {
			t.Errorf("got status %d; want %d", gotStatus, wantStatus)
		}
		gotLen := len(gotResponse.Queue)
		wantLen := 2
		if gotLen != wantLen {
			t.Errorf("got %d queued; want %d", gotLen, wantLen)
		}

		{
			item := gotResponse.Queue[0]
			wantR11nID := queuedTwo1.ID
			gotR11nID := item.ID
			if gotR11nID != wantR11nID {
				t.Errorf("got R11nID %q; want %q", gotR11nID, wantR11nID)
			}
		}
		{
			item := gotResponse.Queue[1]
			wantR11nID := queuedTwo2.ID
			gotR11nID := item.ID
			if gotR11nID != wantR11nID {
				t.Errorf("got R11nID %q; want %q", gotR11nID, wantR11nID)
			}
		}
	})
}

func TestGETDeployQueueHandler_Exchange_errors(t *testing.T) {
	gdh := &GETDeployQueueHandler{
		DeploymentIDErr: fmt.Errorf("this error"),
	}
	_, gotStatus := gdh.Exchange()
	const wantStatus = 404
	if gotStatus != wantStatus {
		t.Errorf("got status %d; want %d", gotStatus, wantStatus)
	}
}

func newR11n(repo string) *sous.Rectification {
	r11n := &sous.Rectification{
		Pair: sous.DeployablePair{},
	}
	r11n.Pair.SetID(newDid(repo))
	return r11n
}
