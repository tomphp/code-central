package factory

import (
	"fmt"
	"io"
	"testing"

	"github.com/tomoram/code-central/core"
)

// The harness cuts a claim before its answer's headers, which the client
// reports as service_unavailable; an answer cut off mid-body, as in CI run
// 43772431843, comes back as the client's wrapped read error.
func TestAClaimsAnswerIsLostUnlessCodeCentralAnswered(t *testing.T) {
	for _, tc := range []struct {
		err  error
		lost bool
	}{
		{nil, false},
		{&core.Error{Code: core.ServiceUnavailable, Message: "cannot reach Code Central"}, true},
		{fmt.Errorf("POST /api/p/acme/claim: reading the answer: %w", io.ErrUnexpectedEOF), true},
		{&core.Error{Code: core.ActionNotAllowed, Message: "acme-2 is held by Claim 1 of another session"}, false},
		{&core.Error{Code: core.NotFound, Message: "no Ticket acme-9"}, false},
		{fmt.Errorf("POST /api/p/acme/claim: HTTP 502: Bad Gateway"), true},
	} {
		if got := answerLost(tc.err); got != tc.lost {
			t.Errorf("answerLost(%v) = %v, want %v", tc.err, got, tc.lost)
		}
	}
}
