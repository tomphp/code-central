package storetest

import (
	"bytes"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/store"
	"github.com/tomoram/code-central/usage"
)

func usageCall(op string) usage.Record {
	return usage.Record{Call: &usage.Call{Time: t0, Op: op, RequestBytes: 10, ResponseBytes: 20}}
}

func usageResponse(ticket string) usage.Record {
	return usage.Record{Response: &usage.Response{
		Time: t0, Ticket: ticket, Claim: 2, SessionID: "s1", Kind: "turn", Model: "claude-opus-5-5",
		Tokens: usage.Tokens{InputTokens: 3, OutputTokens: 4},
	}}
}

func appendUsage(t *testing.T, s store.Store, project, day string, recs ...usage.Record) {
	t.Helper()
	for _, r := range recs {
		if err := s.Usage().Append(project, day, r); err != nil {
			t.Fatalf("append to %s on %s: %v", project, day, err)
		}
	}
}

func readUsage(t *testing.T, s store.Store, project string, days ...string) usage.Records {
	t.Helper()
	recs, err := s.Usage().Read(project, days)
	if err != nil {
		t.Fatalf("read %q over %v: %v", project, days, err)
	}
	return recs
}

func opsOf(recs usage.Records) []string {
	var out []string
	for _, c := range recs.Calls {
		out = append(out, c.Op)
	}
	return out
}

func runUsage(t *testing.T, f Factory) {
	t.Run("usage reads back what was appended, for one project or all", func(t *testing.T) {
		s := f.New(t)
		appendUsage(t, s, "acme", "2026-01-05", usageCall("find_tickets"), usageResponse("acme-1"))
		appendUsage(t, s, "beta", "2026-01-05", usageCall("get_ticket"))

		acme := readUsage(t, s, "acme", "2026-01-05")
		if got, want := opsOf(acme), []string{"find_tickets"}; !slices.Equal(got, want) {
			t.Errorf("acme's calls %v, want %v", got, want)
		}
		if got := acme.Calls[0]; got.RequestBytes != 10 || got.ResponseBytes != 20 || !got.Time.Equal(t0) {
			t.Errorf("call %+v, want the appended sizes and time", got)
		}
		if len(acme.Responses) != 1 {
			t.Fatalf("acme's responses %+v, want 1", acme.Responses)
		}
		resp := acme.Responses[0]
		if resp.Project != "acme" || resp.Day != "2026-01-05" || resp.Ticket != "acme-1" || resp.Claim != 2 ||
			resp.Model != "claude-opus-5-5" || resp.Tokens.InputTokens != 3 || resp.Tokens.OutputTokens != 4 {
			t.Errorf("response %+v, want acme's on 2026-01-05 with its Claim, model and tokens", resp)
		}
		if got, want := opsOf(readUsage(t, s, "", "2026-01-05")), []string{"find_tickets", "get_ticket"}; !slices.Equal(got, want) {
			t.Errorf("every Project's calls %v, want %v", got, want)
		}
		if got := readUsage(t, s, "gamma", "2026-01-05"); len(got.Calls)+len(got.Responses) != 0 {
			t.Errorf("a Project with no usage read %+v", got)
		}
	})

	t.Run("usage reads only the days asked for, in the order asked", func(t *testing.T) {
		s := f.New(t)
		for _, d := range []string{"2026-01-06", "2026-01-05", "2026-01-07"} {
			appendUsage(t, s, "acme", d, usageCall(d))
			appendUsage(t, s, "beta", d, usageCall(d))
		}
		got := opsOf(readUsage(t, s, "", "2026-01-05", "2026-01-06", "2026-01-08"))
		want := []string{"2026-01-05", "2026-01-05", "2026-01-06", "2026-01-06"}
		if !slices.Equal(got, want) {
			t.Errorf("two days of every Project %v, want %v", got, want)
		}
		if got := opsOf(readUsage(t, s, "acme", "2026-01-05")); !slices.Equal(got, []string{"2026-01-05"}) {
			t.Errorf("one day of one Project %v", got)
		}
		if got := readUsage(t, s, "acme"); len(got.Calls) != 0 {
			t.Errorf("no days read %v", opsOf(got))
		}
	})

	t.Run("pruning deletes the days before the cutoff in every project and keeps the rest", func(t *testing.T) {
		s := f.New(t)
		for _, d := range []string{"2026-01-04", "2026-01-05", "2026-01-06"} {
			appendUsage(t, s, "acme", d, usageCall(d))
			appendUsage(t, s, "beta", d, usageCall(d), usageResponse("beta-1"))
		}
		if err := s.Usage().Prune("2026-01-05"); err != nil {
			t.Fatalf("prune: %v", err)
		}
		all := readUsage(t, s, "", "2026-01-04", "2026-01-05", "2026-01-06")
		if got, want := opsOf(all), []string{"2026-01-05", "2026-01-05", "2026-01-06", "2026-01-06"}; !slices.Equal(got, want) {
			t.Errorf("after pruning before 2026-01-05: %v, want %v", got, want)
		}
		if len(all.Responses) != 2 || all.Responses[0].Day != "2026-01-05" {
			t.Errorf("responses %+v, want beta's two, from 2026-01-05 on", all.Responses)
		}
		if err := s.Usage().Prune("2026-01-05"); err != nil {
			t.Errorf("pruning again: %v", err)
		}
	})

	t.Run("usage lists each project's days, oldest first", func(t *testing.T) {
		s := f.New(t)
		for _, d := range []string{"2026-01-06", "2026-01-04", "2026-01-05"} {
			appendUsage(t, s, "acme", d, usageCall(d))
		}
		appendUsage(t, s, "acme", "2026-01-05", usageResponse("acme-1"))
		appendUsage(t, s, "nobody", "2026-01-07", usageResponse("nobody-1"))
		got, err := s.Usage().Days()
		if err != nil {
			t.Fatalf("days: %v", err)
		}
		want := map[string][]string{"acme": {"2026-01-04", "2026-01-05", "2026-01-06"}, "nobody": {"2026-01-07"}}
		if !maps.EqualFunc(got, want, slices.Equal) {
			t.Errorf("days %v, want %v", got, want)
		}
		if err := s.Usage().Prune("2026-01-05"); err != nil {
			t.Fatalf("prune: %v", err)
		}
		got, err = s.Usage().Days()
		if err != nil {
			t.Fatalf("days after pruning: %v", err)
		}
		want["acme"] = []string{"2026-01-05", "2026-01-06"}
		if !maps.EqualFunc(got, want, slices.Equal) {
			t.Errorf("days after pruning before 2026-01-05 %v, want %v", got, want)
		}
	})

	t.Run("an empty store's usage has no days", func(t *testing.T) {
		got, err := f.New(t).Usage().Days()
		if err != nil || len(got) != 0 {
			t.Fatalf("days %v, %v; want none", got, err)
		}
	})

	t.Run("pruning an empty store is not an error", func(t *testing.T) {
		if err := f.New(t).Usage().Prune("2026-01-05"); err != nil {
			t.Fatalf("prune: %v", err)
		}
	})

	t.Run("usage is kept apart from the records", func(t *testing.T) {
		s := f.New(t)
		seedProject(t, s)
		appendUsage(t, s, "acme", "2026-01-05", usageCall("find_tickets"))
		read(t, s, func(r store.Reader) {
			if got := len(r.Projects()); got != 1 {
				t.Errorf("Projects() has %d entries, want 1", got)
			}
		})
		appendUsage(t, s, "nobody", "2026-01-05", usageCall("find_tickets"))
		read(t, s, func(r store.Reader) {
			if _, ok := r.Project("nobody"); ok {
				t.Error("usage made a Project exist")
			}
		})
	})

	if f.Reopen != nil {
		t.Run("usage survives a restart", func(t *testing.T) {
			s := f.New(t)
			seedProject(t, s)
			appendUsage(t, s, "acme", "2026-01-05", usageCall("find_tickets"), usageResponse("acme-1"))
			s = f.Reopen(t, s)
			got := readUsage(t, s, "acme", "2026-01-05")
			if want := []string{"find_tickets"}; !slices.Equal(opsOf(got), want) || len(got.Responses) != 1 {
				t.Errorf("after reopening: calls %v, responses %+v", opsOf(got), got.Responses)
			}
		})
	}

	if f.BreakUsage == nil {
		return
	}
	t.Run("a failing append is logged and dropped, and does not fail the caller", func(t *testing.T) {
		s := f.New(t)
		clk := clock.NewFake(t0)
		var log bytes.Buffer
		r := usage.NewRecorder(s.Usage(), clk, time.UTC, &log)
		r.RecordCall("beta", usage.Call{Time: t0, Op: "before"})

		f.BreakUsage(t, s, "acme")
		r.RecordCall("acme", usage.Call{Time: t0, Op: "find_tickets"})
		r.RecordResponses("acme", []usage.Response{{Time: t0, Ticket: "acme-1"}, {Time: t0, Ticket: "acme-2"}})

		if got := strings.Count(log.String(), "usage: dropped"); got != 3 {
			t.Errorf("the log names %d dropped records, want 3: %q", got, log.String())
		}
		if got := r.Read("acme", 1); len(got.Calls)+len(got.Responses) != 0 {
			t.Errorf("acme's dropped records read back as %+v", got)
		}
		r.RecordCall("beta", usage.Call{Time: t0, Op: "after"})
		if got, want := opsOf(r.Read("beta", 1)), []string{"before", "after"}; !slices.Equal(got, want) {
			t.Errorf("beta's calls %v, want %v: one Project's failure left the others' alone", got, want)
		}
	})
}
