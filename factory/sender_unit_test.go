package factory

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tomoram/code-central/client"
	"github.com/tomoram/code-central/core"
)

func TestAnOlderCodeCentralsNotFoundForUsageIsDroppedNeverRetried(t *testing.T) {
	var posts atomic.Int32
	old := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts.Add(1)
		http.NotFound(w, r)
	}))
	defer old.Close()
	s := newSender(client.New(old.URL).PostUsage, time.Millisecond)
	go s.run()
	s.add("s1", core.UsageResponse{Kind: "implementation"})
	for posts.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond)
	s.close()
	if n := posts.Load(); n != 1 {
		t.Errorf("%d posts; want the one batch posted once and dropped", n)
	}
}
