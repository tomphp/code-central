package core

import (
	"context"
	_ "embed"
	"fmt"
	"slices"
	"strings"

	"github.com/tomoram/code-central/store"
)

// Import (ticket 40). An agent maps a repository's whole .scratch/ folder
// into one payload, following get_import_guide; core checks it strictly
// and writes it all at once. The service never reads a repository.

// ImportInput is import's input.
type ImportInput struct {
	Payload ImportPayload `json:"payload"`
	// DryRun runs every check and returns the plan, writing nothing.
	DryRun bool `json:"dry_run,omitempty"`
}

// ImportPayload is a whole .scratch/ folder: one Spec per feature folder,
// and every Ticket in it. Items refer to each other by source path.
type ImportPayload struct {
	Specs   []ImportSpec   `json:"specs"`
	Tickets []ImportTicket `json:"tickets"`
}

// ImportSpec is one feature folder. Source becomes the Spec's source Tag.
type ImportSpec struct {
	Source string            `json:"source"`
	Title  string            `json:"title"`
	Body   string            `json:"body,omitempty"`
	Labels []string          `json:"labels,omitempty"`
	Tags   map[string]string `json:"tags,omitempty"`
}

// ImportTicket is one Scratch file. Spec is its Spec's source; BlockedBy
// and Replacements are other Tickets' sources in the same payload. State
// defaults to Draft; Comment, when given, is stored as one comment.
type ImportTicket struct {
	Source       string            `json:"source"`
	Spec         string            `json:"spec"`
	Title        string            `json:"title"`
	State        store.State       `json:"state,omitempty"`
	Description  string            `json:"description,omitempty"`
	Labels       []string          `json:"labels,omitempty"`
	Tags         map[string]string `json:"tags,omitempty"`
	BlockedBy    []string          `json:"blocked_by,omitempty"`
	Replacements []string          `json:"replacements,omitempty"`
	Comment      string            `json:"comment,omitempty"`
}

// ImportResult is the plan (on a dry run, with provisional IDs) or what
// was created, grouped by Spec, with a count per State.
type ImportResult struct {
	DryRun      bool                `json:"dry_run"`
	Specs       []ImportedSpec      `json:"specs"`
	StateCounts map[store.State]int `json:"state_counts"`
	NextStep    string              `json:"next_step"`
}

// ImportedSpec is one Spec of an Import's plan or result.
type ImportedSpec struct {
	Source  string            `json:"source"`
	ID      string            `json:"id"`
	Title   string            `json:"title"`
	Labels  []string          `json:"labels"`
	Tags    map[string]string `json:"tags"`
	Tickets []ImportedTicket  `json:"tickets"`
}

// ImportedTicket is one Ticket of an Import's plan or result.
type ImportedTicket struct {
	Source       string            `json:"source"`
	ID           string            `json:"id"`
	Title        string            `json:"title"`
	State        store.State       `json:"state"`
	Labels       []string          `json:"labels"`
	Tags         map[string]string `json:"tags"`
	BlockedBy    []string          `json:"blocked_by"`
	Replacements []string          `json:"replacements"`
}

// ImportGuideView is get_import_guide's result.
type ImportGuideView struct {
	Guide string `json:"guide"`
}

//go:embed import-guide.md
var importGuide string

// importNextSteps are import's next steps, dry run first.
var importNextSteps = map[string]string{
	"import (dry run)": "Show the human this plan with every doubt you found, and import again without dry_run only after their go-ahead.",
	"import":           "Delete `.scratch/` and commit that, so Code Central is the one source of truth for this work.",
}

// GetImportGuide is get_import_guide: how to map a .scratch/ folder.
func (c *Core) GetImportGuide(ctx context.Context, key string) (ImportGuideView, error) {
	if _, err := c.GetProject(ctx, key); err != nil {
		return ImportGuideView{}, err
	}
	return ImportGuideView{Guide: importGuide}, nil
}

// importStates are the States an Import may create a Ticket in. Only
// Import can create one straight into Done or Cancelled.
var importStates = []store.State{store.Draft, store.Ready, store.Done, store.Cancelled}

// Import is import(payload, dry_run?). It writes as agent whoever calls,
// never creates a Project, and writes everything or nothing.
func (c *Core) Import(_ context.Context, key string, in ImportInput) (ImportResult, error) {
	p := in.Payload
	if err := checkImport(p); err != nil {
		return ImportResult{}, err
	}
	var res ImportResult
	plan := func(r store.Reader) (store.Import, error) {
		if err := writable(r, key); err != nil {
			return store.Import{}, err
		}
		if err := alreadyImported(r, key, p); err != nil {
			return store.Import{}, err
		}
		var im store.Import
		im, res = c.buildImport(r, key, p)
		return im, importCycles(r, key, im)
	}
	var err error
	if in.DryRun {
		// A dry run stores no pending plan.
		err = c.store.View(func(r store.Reader) error { _, err := plan(r); return err })
		res.NextStep = importNextSteps["import (dry run)"]
	} else {
		err = c.store.Update(func(w store.Writer) error {
			im, err := plan(w)
			if err != nil {
				return err
			}
			return w.PutImport(key, im)
		})
		res.NextStep = importNextSteps["import"]
	}
	if err != nil {
		return ImportResult{}, err
	}
	res.DryRun = in.DryRun
	return res, nil
}

// checkImport validates the payload on its own, naming the first fault's
// field path.
func checkImport(p ImportPayload) error {
	if len(p.Specs) == 0 {
		return invalid("specs", "an Import needs at least one Spec: one per feature folder")
	}
	sources := map[string]string{} // source → the field that first gave it
	specs := map[string]bool{}
	tickets := map[string]bool{}
	source := func(field, s string) error {
		if !singleLine(s) {
			return invalid(field, "a source is the file's path from the repository root, non-empty and single-line")
		}
		if first, ok := sources[s]; ok {
			return invalid(field, "source %q is already used by %s", s, first)
		}
		sources[s] = field
		return nil
	}
	for i, s := range p.Specs {
		at := fmt.Sprintf("specs[%d]", i)
		if err := source(at+".source", s.Source); err != nil {
			return err
		}
		specs[s.Source] = true
		if strings.TrimSpace(s.Title) == "" {
			return invalid(at+".title", "a Spec needs a title: the folder's name when it has no spec.md or map.md")
		}
		if err := checkImportLabelsTags(at, s.Labels, s.Tags); err != nil {
			return err
		}
	}
	for i, t := range p.Tickets {
		at := fmt.Sprintf("tickets[%d]", i)
		if err := source(at+".source", t.Source); err != nil {
			return err
		}
		tickets[t.Source] = true
	}
	for i, t := range p.Tickets {
		at := fmt.Sprintf("tickets[%d]", i)
		if strings.TrimSpace(t.Title) == "" {
			return invalid(at+".title", "a Ticket needs a title")
		}
		if !specs[t.Spec] {
			return invalid(at+".spec", "%q is not the source of a Spec in this payload", t.Spec)
		}
		if t.State != "" && !slices.Contains(importStates, t.State) {
			return invalid(at+".state", "an imported Ticket is Draft, Ready, Done or Cancelled, not %q", t.State)
		}
		if err := checkImportLabelsTags(at, t.Labels, t.Tags); err != nil {
			return err
		}
		for j, b := range t.BlockedBy {
			if !tickets[b] {
				return invalid(fmt.Sprintf("%s.blocked_by[%d]", at, j), "%q is not the source of a Ticket in this payload; drop the link and list it as a doubt", b)
			}
		}
		if len(t.Replacements) > 0 && t.State != store.Cancelled {
			return invalid(at+".replacements", "only a Cancelled Ticket has Replacements")
		}
		for j, r := range t.Replacements {
			field := fmt.Sprintf("%s.replacements[%d]", at, j)
			if !tickets[r] {
				return invalid(field, "%q is not the source of a Ticket in this payload", r)
			}
			if r == t.Source {
				return invalid(field, "a Ticket can't replace itself")
			}
		}
	}
	return nil
}

func checkImportLabelsTags(at string, labels []string, tags map[string]string) error {
	for j, l := range labels {
		if !singleLine(l) {
			return invalid(fmt.Sprintf("%s.labels[%d]", at, j), "a Label is non-empty and single-line, not %q", l)
		}
	}
	keys := make([]string, 0, len(tags))
	for k := range tags {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		field := at + ".tags." + k
		if !tagKey.MatchString(k) {
			return invalid(field, "Tag key %q must be lowercase letters, digits, '.', '_' and '-'", k)
		}
		if k == "source" {
			return invalid(field, "the source Tag is set from source; don't give it as a Tag")
		}
		if !singleLine(tags[k]) {
			return invalid(field, "Tag %q needs a non-empty, single-line value", k)
		}
	}
	return nil
}

func singleLine(s string) bool { return strings.TrimSpace(s) != "" && !strings.ContainsAny(s, "\r\n") }

// alreadyImported refuses an Import if any Spec or Ticket in the Project
// already has a source Tag in the payload, listing every match.
func alreadyImported(r store.Reader, key string, p ImportPayload) error {
	existing := map[string]int{}
	for _, s := range r.Specs(key) {
		if src, ok := s.Tags["source"]; ok {
			existing[src] = s.Number
		}
	}
	for _, t := range r.Tickets(key) {
		if src, ok := t.Tags["source"]; ok {
			existing[src] = t.Number
		}
	}
	var field string
	var matches []string
	check := func(f, src string) {
		if n, ok := existing[src]; ok {
			if field == "" {
				field = f
			}
			matches = append(matches, fmt.Sprintf("%s (%s)", src, DisplayID(key, n)))
		}
	}
	for i, s := range p.Specs {
		check(fmt.Sprintf("specs[%d].source", i), s.Source)
	}
	for i, t := range p.Tickets {
		check(fmt.Sprintf("tickets[%d].source", i), t.Source)
	}
	if len(matches) == 0 {
		return nil
	}
	return invalid(field, "this Project already has %d of these sources, so this Import has run before; nothing was imported: %s",
		len(matches), strings.Join(matches, ", "))
}

// buildImport numbers every item from the counter in source order (each
// Spec, then its Tickets) and builds the records and the plan.
func (c *Core) buildImport(r store.Reader, key string, p ImportPayload) (store.Import, ImportResult) {
	now := c.now()
	next := r.NextNumber(key)
	numbers := map[string]int{}
	var order []int // payload index of each Ticket, in number order
	for _, s := range p.Specs {
		numbers[s.Source] = next
		next++
		for i, t := range p.Tickets {
			if t.Spec == s.Source {
				numbers[t.Source] = next
				next++
				order = append(order, i)
			}
		}
	}
	refs := func(sources []string) []int {
		var out []int
		for _, s := range sources {
			if n := numbers[s]; !slices.Contains(out, n) {
				out = append(out, n)
			}
		}
		return out
	}
	withSource := func(tags map[string]string, src string) map[string]string {
		out := map[string]string{"source": src}
		for k, v := range tags {
			out[k] = v
		}
		return out
	}

	var im store.Import
	res := ImportResult{StateCounts: map[store.State]int{}}
	bySpec := map[int]int{} // Spec number → index in res.Specs
	for _, s := range p.Specs {
		sp := store.Spec{Number: numbers[s.Source], Title: s.Title, Labels: s.Labels, Tags: withSource(s.Tags, s.Source),
			Created: now, Updated: now, Body: s.Body}
		im.Specs = append(im.Specs, sp)
		bySpec[sp.Number] = len(res.Specs)
		res.Specs = append(res.Specs, ImportedSpec{Source: s.Source, ID: DisplayID(key, sp.Number), Title: sp.Title,
			Labels: orEmpty(sp.Labels), Tags: sp.Tags, Tickets: []ImportedTicket{}})
	}
	for _, i := range order {
		t := p.Tickets[i]
		state := t.State
		if state == "" {
			state = store.Draft
		}
		tk := store.Ticket{
			Number: numbers[t.Source], Title: t.Title, State: state, Spec: numbers[t.Spec],
			Labels: t.Labels, Tags: withSource(t.Tags, t.Source), BlockedBy: refs(t.BlockedBy), Replacements: refs(t.Replacements),
			Created: now, Updated: now, Description: t.Description,
			Moves: []store.Move{{To: state, Actor: string(Agent), Time: now, Via: "import"}},
		}
		if t.Comment != "" {
			addComment(&tk, Agent, now, 0, t.Comment)
		}
		im.Tickets = append(im.Tickets, tk)
		res.StateCounts[state]++
		g := &res.Specs[bySpec[tk.Spec]]
		g.Tickets = append(g.Tickets, ImportedTicket{
			Source: t.Source, ID: DisplayID(key, tk.Number), Title: tk.Title, State: state,
			Labels: orEmpty(tk.Labels), Tags: tk.Tags,
			BlockedBy: displayIDs(key, tk.BlockedBy), Replacements: displayIDs(key, tk.Replacements),
		})
	}
	return im, res
}

// importCycles rejects an Import in which a Ticket would wait on itself,
// through Blockers or the Replacements of Cancelled ones, or whose
// Replacements lead back to the Ticket they replace.
func importCycles(r store.Reader, key string, im store.Import) error {
	ov := importOverlay{Reader: r, key: key, tickets: map[int]store.Ticket{}}
	for _, t := range im.Tickets {
		ov.tickets[t.Number] = t
	}
	for _, t := range im.Tickets {
		self := func(n int) bool { return n == t.Number }
		if path := waitPath(ov, key, waitsOn(ov, key, t.Number), self); path != nil {
			return cycleError(key, append([]int{t.Number}, path...))
		}
		if path := replacementPath(ov, key, t.Replacements, t.Number, map[int]bool{}); path != nil {
			return &Error{Code: Cycle, Message: "the Replacements form a loop: " + strings.Join(displayIDs(key, append([]int{t.Number}, path...)), " → ") + " (each is replaced by the next)"}
		}
	}
	return nil
}

// replacementPath finds a chain of Replacements from one of reps back to
// target, or nil.
func replacementPath(r store.Reader, key string, reps []int, target int, seen map[int]bool) []int {
	for _, n := range reps {
		if n == target {
			return []int{n}
		}
		if seen[n] {
			continue
		}
		seen[n] = true
		if t, ok := r.Ticket(key, n); ok && t.State == store.Cancelled {
			if p := replacementPath(r, key, t.Replacements, target, seen); p != nil {
				return append([]int{n}, p...)
			}
		}
	}
	return nil
}

// importOverlay is a Reader that also sees an Import's new Tickets.
type importOverlay struct {
	store.Reader
	key     string
	tickets map[int]store.Ticket
}

func (o importOverlay) Ticket(project string, n int) (store.Ticket, bool) {
	if t, ok := o.tickets[n]; ok && project == o.key {
		return t, true
	}
	return o.Reader.Ticket(project, n)
}
