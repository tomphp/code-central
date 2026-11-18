package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/httpapi"
	"github.com/tomoram/code-central/ops"
)

// recordingService is an ops.Service that records each call it
// dispatches and answers with answer and err; any other method panics.
type recordingService struct {
	ops.Service
	calls  []ops.Call
	answer any
	err    error
}

func (s *recordingService) Dispatch(_ context.Context, call ops.Call) (any, error) {
	s.calls = append(s.calls, call)
	return s.answer, s.err
}

// The HTTP door depends on ops.Service alone: an operation's request
// becomes one Call, with its path, body and Actor header, and the
// dispatch's result or error is the response.
func TestTheHTTPDoorDispatchesEachOperationThroughItsService(t *testing.T) {
	svc := &recordingService{answer: map[string]string{"id": "acme-2"}}
	mux := http.NewServeMux()
	httpapi.Register(mux, svc)

	req := httptest.NewRequest("POST", "/api/p/acme/tickets/2/comment", strings.NewReader(`{"body":"Looks good."}`))
	req.Header.Set(httpapi.ActorHeader, "human")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `{"id":"acme-2"}` {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body)
	}
	if len(svc.calls) != 1 {
		t.Fatalf("%d calls, want 1", len(svc.calls))
	}
	got := svc.calls[0]
	want := ops.Call{Op: ops.Comment, Key: "acme", In: &core.CommentInput{Ticket: "2", Body: "Looks good."}, Actor: core.Human}
	if got.Op != want.Op || got.Key != want.Key || got.Actor != want.Actor || got.Args != nil || !reflect.DeepEqual(got.In, want.In) {
		t.Fatalf("call %+v, want %+v", got, want)
	}

	svc.err = &core.Error{Code: core.StaleClaim, Message: "Your Claim has ended."}
	rec = do(mux, "POST", "/api/p/acme/tickets/2/comment", `{"body":"Again."}`)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"code":"stale_claim"`) {
		t.Fatalf("error: status %d, body %s", rec.Code, rec.Body)
	}
	if got := svc.calls[1].Actor; got != "" {
		t.Fatalf("no Actor header: Actor %q, want none named", got)
	}
}
