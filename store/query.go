package store

import (
	"cmp"
	"iter"
	"maps"
	"slices"

	"github.com/tomoram/code-central/domain"
)

// The Reader's queries (Spec code-central-1032): core reads the Tickets
// and Specs it needs through one filter per record kind, so a database
// Store answers each with an index instead of loading a Project.

// TicketFilter picks the Tickets FindTickets returns. Its fields combine
// with AND, and a field left zero picks every Ticket.
type TicketFilter struct {
	// Spec picks the Tickets of any of these Specs.
	Spec []int
	// States picks the Tickets in any of these States.
	States []domain.State
	// Label picks the Tickets carrying this Label.
	Label string
	// Tag picks the Tickets carrying its Tag.
	Tag *TagFilter
	// Link picks the Tickets linked as it says.
	Link *LinkFilter
	// PushedCommit picks the Tickets that pushed its commit in its
	// Repository.
	PushedCommit *PushedCommitFilter
	// PushedBranch picks the Tickets that pushed a commit as its branch
	// in its Repository.
	PushedBranch *PushedBranchFilter
	// HasPushes picks the Tickets that pushed any commit.
	HasPushes bool
	// BlockedBy picks the Tickets with this Ticket among their Blockers.
	BlockedBy int
	// ReplacedBy picks the Tickets with this Ticket among their
	// Replacements.
	ReplacedBy int
	// Parts are the History parts each Ticket is read with, as Ticket
	// reads them: none, for a list of heads.
	Parts []Part
	// Order is the order the Tickets come in.
	Order Order
}

// SpecFilter picks the Specs FindSpecs returns, as TicketFilter picks
// Tickets.
type SpecFilter struct {
	Label string
	Tag   *TagFilter
	Link  *LinkFilter
}

// TagFilter picks the records whose Tag Key is set to any of Values, or
// to any value at all when Values is empty.
type TagFilter struct {
	Key    string
	Values []string
}

// LinkFilter picks the records linked by Integration, detached or not:
// to the external item ID, or to any item when ID is empty.
type LinkFilter struct {
	Integration string
	ID          string
}

// PushedCommitFilter names a commit pushed in a Repository, as a
// PushedCommit records them: Repository is empty for the Home
// Repository.
type PushedCommitFilter struct {
	Repository string
	Commit     string
}

// PushedBranchFilter names a branch pushed in a Repository, as a
// PushedCommit records them: Repository is empty for the Home
// Repository.
type PushedBranchFilter struct {
	Repository string
	Branch     string
}

// Order is the order FindTickets returns Tickets in.
type Order int

const (
	// ByNumber is number order.
	ByNumber Order = iota
	// ByClaim is claim order: Created, then Number.
	ByClaim
)

// compare compares a and b in o.
func (o Order) compare(a, b domain.Ticket) int {
	if o == ByClaim {
		if c := a.Created.Compare(b.Created); c != 0 {
			return c
		}
	}
	return cmp.Compare(a.Number, b.Number)
}

// picks reports whether f picks t.
func (f TicketFilter) picks(t domain.Ticket) bool {
	pushed := func(match func(domain.PushedCommit) bool) bool { return slices.ContainsFunc(t.PushedCommits, match) }
	switch {
	case len(f.Spec) > 0 && !slices.Contains(f.Spec, t.Spec),
		len(f.States) > 0 && !slices.Contains(f.States, t.State),
		!picksMarks(f.Label, f.Tag, f.Link, t.Labels, t.Tags, t.Link),
		f.PushedCommit != nil && !pushed(func(pc domain.PushedCommit) bool {
			return pc.Repository == f.PushedCommit.Repository && pc.Commit == f.PushedCommit.Commit
		}),
		f.PushedBranch != nil && !pushed(func(pc domain.PushedCommit) bool {
			return pc.Repository == f.PushedBranch.Repository && pc.Branch == f.PushedBranch.Branch
		}),
		f.HasPushes && len(t.PushedCommits) == 0,
		f.BlockedBy != 0 && !slices.Contains(t.BlockedBy, f.BlockedBy),
		f.ReplacedBy != 0 && !slices.Contains(t.Replacements, f.ReplacedBy):
		return false
	}
	return true
}

// picksMarks reports whether a record with labels, tags and link carries
// label, tag and link, those of them given.
func picksMarks(label string, tag *TagFilter, link *LinkFilter, labels []string, tags map[string]string, l domain.Link) bool {
	if label != "" && !slices.Contains(labels, label) {
		return false
	}
	if tag != nil {
		v, ok := tags[tag.Key]
		if !ok || len(tag.Values) > 0 && !slices.Contains(tag.Values, v) {
			return false
		}
	}
	return link == nil || l.Integration == link.Integration && (link.ID == "" || l.ID == link.ID)
}

func (r reader) FindTickets(project string, f TicketFilter) []domain.Ticket {
	out := []domain.Ticket{}
	p, ok := r.x.projects[project]
	if !ok {
		return out
	}
	for _, t := range p.tickets {
		if f.picks(t) {
			out = append(out, readOf(t, f.Parts))
		}
	}
	slices.SortFunc(out, f.Order.compare)
	return out
}

func (r reader) FindSpecs(project string, f SpecFilter) []domain.Spec {
	out := []domain.Spec{}
	p, ok := r.x.projects[project]
	if !ok {
		return out
	}
	for _, s := range p.specs {
		if picksMarks(f.Label, f.Tag, f.Link, s.Labels, s.Tags, s.Link) {
			out = append(out, cloneSpec(s))
		}
	}
	slices.SortFunc(out, func(a, b domain.Spec) int { return cmp.Compare(a.Number, b.Number) })
	return out
}

func (r reader) StateCounts(project string) map[domain.State]int {
	out := map[domain.State]int{}
	if p, ok := r.x.projects[project]; ok {
		for _, t := range p.tickets {
			out[t.State]++
		}
	}
	return out
}

// SessionClaim is a Claim ClaimBySession found, with the Project and the
// number of the Ticket it is on.
type SessionClaim struct {
	Project string
	Ticket  int
	Claim   domain.Claim
}

func (r reader) ClaimBySession(sessionID string) (SessionClaim, bool) {
	var out SessionClaim
	found := false
	if sessionID == "" {
		return out, false
	}
	for _, p := range r.Projects() {
		tickets := r.x.projects[p.Key].tickets
		for _, n := range slices.Sorted(maps.Keys(tickets)) {
			for _, cl := range tickets[n].Claims {
				if cl.SessionID == sessionID && (!found || cl.Started.After(out.Claim.Started)) {
					out, found = SessionClaim{Project: p.Key, Ticket: n, Claim: cl}, true
				}
			}
		}
	}
	return out, found
}

// TicketCommit is one commit TicketCommits lists: a Ticket of Spec
// pushed it, or landed at it, in the Repository named.
type TicketCommit struct {
	Ticket int
	Spec   int
	// Repository is the Repository Name; empty for the Home Repository.
	Repository string
	Commit     string
	Kind       CommitKind
}

// CommitKind is whether a TicketCommit was pushed or landed at.
type CommitKind string

// The kinds of TicketCommit.
const (
	CommitPushed CommitKind = "pushed"
	CommitLanded CommitKind = "landed"
)

func (r reader) TicketCommits(project string) []TicketCommit {
	out := []TicketCommit{}
	p, ok := r.x.projects[project]
	if !ok {
		return out
	}
	for _, n := range slices.Sorted(maps.Keys(p.tickets)) {
		t := p.tickets[n]
		row := func(repository, commit string, kind CommitKind) {
			out = append(out, TicketCommit{Ticket: n, Spec: t.Spec, Repository: repository, Commit: commit, Kind: kind})
		}
		for _, pc := range t.PushedCommits {
			row(pc.Repository, pc.Commit, CommitPushed)
		}
		for _, m := range t.Moves {
			if m.Commit != "" {
				row("", m.Commit, CommitLanded)
			}
		}
		// The Home Repository, named by the Project Key, landed at its
		// Moves' commits.
		for _, cr := range t.Repositories {
			if cr.Landed != nil && cr.Name != project {
				row(cr.Name, cr.Landed.Commit, CommitLanded)
			}
		}
	}
	return out
}

// Record is one record Records reads: a Spec or a Ticket, the other nil.
type Record struct {
	Spec   *domain.Spec
	Ticket *domain.Ticket
}

func (r reader) Records(project string) iter.Seq[Record] {
	return func(yield func(Record) bool) {
		for _, s := range r.Specs(project) {
			if !yield(Record{Spec: &s}) {
				return
			}
		}
		for _, t := range r.Tickets(project, History...) {
			if !yield(Record{Ticket: &t}) {
				return
			}
		}
	}
}
