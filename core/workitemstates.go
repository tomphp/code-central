package core

import (
	"strings"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
)

func workItemStatesView(s domain.WorkItemStates) api.WorkItemStates {
	return api.WorkItemStates{InProgress: s.InProgress.Name, Accepted: s.Accepted.Name, Done: s.Done.Name}
}

// namedWorkItemStates are the States in as given: trimmed, not otherwise
// checked, with no since times yet.
func namedWorkItemStates(in api.WorkItemStates) domain.WorkItemStates {
	name := func(s string) domain.WorkItemState { return domain.WorkItemState{Name: strings.TrimSpace(s)} }
	return domain.WorkItemStates{InProgress: name(in.InProgress), Accepted: name(in.Accepted), Done: name(in.Done)}
}

// withSince dates each mapping of named: now when it goes from blank to
// non-blank, the old since when it stays non-blank (a corrected name is
// still the mapping turned on then), none when blank.
func withSince(named, was domain.WorkItemStates, now time.Time) domain.WorkItemStates {
	date := func(n, w domain.WorkItemState) domain.WorkItemState {
		switch {
		case n.Name == "":
			return domain.WorkItemState{}
		case w.Name == "":
			n.Since = now
		default:
			n.Since = w.Since
		}
		return n
	}
	return domain.WorkItemStates{InProgress: date(named.InProgress, was.InProgress), Accepted: date(named.Accepted, was.Accepted), Done: date(named.Done, was.Done)}
}
