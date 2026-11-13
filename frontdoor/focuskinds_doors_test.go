package frontdoor_test

import (
	"reflect"
	"testing"

	"github.com/tomoram/code-central/core"
)

// Focus Kinds (Spec code-central-550): set_focus_kinds is the human's, so
// HTTP only.
func init() {
	cases["set_focus_kinds"] = opCase{
		ok: call{http: setFocusKinds(`{"off":["in_review","answered"]}`)},
		check: func(t *testing.T, out map[string]any) {
			acknowledgement("acme", "focus_kinds")(t, out)
		},
		full: func(t *testing.T, out map[string]any) {
			t.Helper()
			if off := focusKindsOff(t, out); !reflect.DeepEqual(off, []any{"answered", "in_review"}) {
				t.Fatalf("focus_kinds.off = %v, want [answered in_review] in Needs Me order", off)
			}
		},
		bad:  call{http: setFocusKinds(`{"off":["lunch"]}`)},
		code: core.Invalid,
	}
}

func setFocusKinds(body string) httpCall {
	return httpCall{"POST", "/api/p/acme/set_focus_kinds", body}
}

// focusKindsOff is a Project view's focus_kinds.off.
func focusKindsOff(t *testing.T, p map[string]any) []any {
	t.Helper()
	fk, ok := p["focus_kinds"].(map[string]any)
	if !ok {
		t.Fatalf("focus_kinds = %v, want it filled in", p["focus_kinds"])
	}
	off, ok := fk["off"].([]any)
	if !ok {
		t.Fatalf("focus_kinds.off = %v, want a list", fk["off"])
	}
	return off
}

func TestEveryProjectReadCarriesFocusKindsWithEveryReasonOn(t *testing.T) {
	srv := newServer(t)
	p := httpOK(t, srv, httpCall{"GET", "/api/projects/acme", ""})
	if off := focusKindsOff(t, p); len(off) != 0 {
		t.Fatalf("focus_kinds.off = %v, want empty", off)
	}
	rows, _ := httpOK(t, srv, httpCall{"GET", "/api/projects", ""})["rows"].([]any)
	if off := focusKindsOff(t, rows[0].(map[string]any)); len(off) != 0 {
		t.Fatalf("listed focus_kinds.off = %v, want empty", off)
	}
}

func TestSetFocusKindsReplacesTheListWholeAndCollapsesDuplicates(t *testing.T) {
	srv := newServer(t)
	httpOK(t, srv, setFocusKinds(`{"off":["in_review","needs_answer"]}`))
	out := httpOK(t, srv, setFocusKinds(`{"off":["failed","failed"],"full":true}`))
	if off := focusKindsOff(t, out); !reflect.DeepEqual(off, []any{"failed"}) {
		t.Fatalf("returned focus_kinds.off = %v, want [failed]", off)
	}
	p := httpOK(t, srv, httpCall{"GET", "/api/projects/acme", ""})
	if off := focusKindsOff(t, p); !reflect.DeepEqual(off, []any{"failed"}) {
		t.Fatalf("read back focus_kinds.off = %v, want [failed]", off)
	}
	httpOK(t, srv, setFocusKinds(`{"off":[]}`))
	p = httpOK(t, srv, httpCall{"GET", "/api/projects/acme", ""})
	if off := focusKindsOff(t, p); len(off) != 0 {
		t.Fatalf("focus_kinds.off = %v, want every reason back on", off)
	}
}

func TestSetFocusKindsRefusesAnUnknownReasonNamingTheField(t *testing.T) {
	srv := newServer(t)
	e := httpRejected(t, srv, setFocusKinds(`{"off":["answered","lunch"]}`), core.Invalid)
	if e.Field != "off" {
		t.Fatalf("field = %q, want off (%s)", e.Field, e.Message)
	}
	p := httpOK(t, srv, httpCall{"GET", "/api/projects/acme", ""})
	if off := focusKindsOff(t, p); len(off) != 0 {
		t.Fatalf("focus_kinds.off = %v, want unchanged", off)
	}
}

func TestSetFocusKindsIsRejectedForAnAgent(t *testing.T) {
	srv := newServer(t)
	status, b := httpDoAs(t, srv, core.Agent, setFocusKinds(`{"off":["answered"]}`))
	errorJSON(t, b, core.ActionNotAllowed)
	if status == 200 {
		t.Fatalf("status %d, want a refusal", status)
	}
}

// Focus Kinds act only on the web's focus queue: Needs Me and its count
// stay as they were with a reason off.
func TestAReasonOffLeavesNeedsMeAndItsCountUnchanged(t *testing.T) {
	srv := newServer(t)
	ticketInReview(t, srv)
	before := httpOK(t, srv, httpCall{"GET", "/api/needs-me", ""})
	beforeRows, _ := httpOK(t, srv, httpCall{"GET", "/api/projects", ""})["rows"].([]any)
	httpOK(t, srv, setFocusKinds(`{"off":["in_review"]}`))
	after := httpOK(t, srv, httpCall{"GET", "/api/needs-me", ""})
	if !reflect.DeepEqual(after, before) || after["count"] != float64(1) {
		t.Fatalf("needs-me with In Review off = %v, want %v", after, before)
	}
	afterRows, _ := httpOK(t, srv, httpCall{"GET", "/api/projects", ""})["rows"].([]any)
	if got, want := afterRows[0].(map[string]any)["needs_me"], beforeRows[0].(map[string]any)["needs_me"]; got != want || got != float64(1) {
		t.Fatalf("acme's needs_me = %v, want %v", got, want)
	}
	if v := httpOK(t, srv, httpCall{"GET", "/api/p/acme/tickets/3", ""}); v["needs_me_reason"] != "in_review" {
		t.Fatalf("acme-3 needs_me_reason = %v, want in_review", v["needs_me_reason"])
	}
}
