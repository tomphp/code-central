package core

import (
	"context"
	_ "embed"
	"fmt"
	"slices"
	"strings"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// Import (ticket 40). An agent maps a repository's whole .scratch/ folder
// into one payload, following get_import_guide; core checks it strictly
// and writes it all at once. The service never reads a repository.

//go:embed import-guide.md
var importGuide string

// importNextSteps are import's next steps, dry run first.
var importNextSteps = map[string]string{
	"import (dry run)": "Show the human this plan with every doubt you found, and import again without dry_run only after their go-ahead.",
	"import":           "Delete `.scratch/` and commit that, so Code Central is the one source of truth for this work.",
}

// GetImportGuide is get_import_guide: how to map a .scratch/ folder.
func (c *Core) GetImportGuide(ctx context.Context, key string) (api.ImportGuideView, error) {
	if _, err := c.GetProject(ctx, key); err != nil {
		return api.ImportGuideView{}, err
	}
	return api.ImportGuideView{Guide: importGuide}, nil
}

// importStates are the States an Import may create a Ticket in. Only
// Import can create one straight into Done or Cancelled.
var importStates = []domain.State{domain.Draft, domain.Ready, domain.Done, domain.Cancelled}

// Import is import(payload, dry_run?). It writes as agent whoever calls,
// never creates a Project, and writes everything or nothing.
func (c *Core) Import(ctx context.Context, key string, in api.ImportInput) (api.ImportResult, error) {
	p := in.Payload
	if err := checkImport(ctx, p); err != nil {
		return api.ImportResult{}, err
	}
	var res api.ImportResult
	plan := func(r store.Reader) (store.Import, error) {
		if err := writable(r, key); err != nil {
			return store.Import{}, err
		}
		if err := alreadyImported(r, key, p); err != nil {
			return store.Import{}, err
		}
		if err := importBranchTags(r, key, p); err != nil {
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
		return api.ImportResult{}, err
	}
	res.DryRun = in.DryRun
	return res, nil
}

// checkImport validates the payload on its own, naming the first fault's
// field path. Only the human may name the auto-accept Label.
func checkImport(ctx context.Context, p api.ImportPayload) error {
	if len(p.Specs) == 0 {
		return api.InvalidField("specs", "an Import needs at least one Spec: one per feature folder")
	}
	sources := map[string]string{} // source → the field that first gave it
	specs := map[string]bool{}
	tickets := map[string]bool{}
	source := func(field, s string) error {
		if !singleLine(s) {
			return api.InvalidField(field, "a source is the file's path from the repository root, non-empty and single-line")
		}
		if first, ok := sources[s]; ok {
			return api.InvalidField(field, "source %q is already used by %s", s, first)
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
			return api.InvalidField(at+".title", "a Spec needs a title: the folder's name when it has no spec.md or map.md")
		}
		if err := checkImportLabelsTags(ctx, at, s.Labels, s.Tags, true); err != nil {
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
			return api.InvalidField(at+".title", "a Ticket needs a title")
		}
		if !specs[t.Spec] {
			return api.InvalidField(at+".spec", "%q is not the source of a Spec in this payload", t.Spec)
		}
		if t.State != "" && !slices.Contains(importStates, t.State) {
			return api.InvalidField(at+".state", "an imported Ticket is Draft, Ready, Done or Cancelled, not %q", t.State)
		}
		if err := checkImportLabelsTags(ctx, at, t.Labels, t.Tags, false); err != nil {
			return err
		}
		for j, b := range t.BlockedBy {
			if !tickets[b] {
				return api.InvalidField(fmt.Sprintf("%s.blocked_by[%d]", at, j), "%q is not the source of a Ticket in this payload; drop the link and list it as a doubt", b)
			}
		}
		if len(t.Replacements) > 0 && t.State != domain.Cancelled {
			return api.InvalidField(at+".replacements", "only a Cancelled Ticket has Replacements")
		}
		for j, r := range t.Replacements {
			field := fmt.Sprintf("%s.replacements[%d]", at, j)
			if !tickets[r] {
				return api.InvalidField(field, "%q is not the source of a Ticket in this payload", r)
			}
			if r == t.Source {
				return api.InvalidField(field, "a Ticket can't replace itself")
			}
		}
	}
	return nil
}

func checkImportLabelsTags(ctx context.Context, at string, labels []string, tags map[string]string, onSpec bool) error {
	for j, l := range labels {
		field := fmt.Sprintf("%s.labels[%d]", at, j)
		if !singleLine(l) {
			return api.InvalidField(field, "a Label is non-empty and single-line, not %q", l)
		}
		if err := checkReservedNames(field, []string{l}); err != nil {
			return err
		}
		if err := checkHumanLabels(ctx, field, []string{l}); err != nil {
			return err
		}
		if err := checkPlacement(field, []string{l}, onSpec); err != nil {
			return err
		}
	}
	if !onSpec {
		if err := checkOneType(at+".labels", labels); err != nil {
			return err
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
			return api.InvalidField(field, "Tag key %q must be lowercase letters, digits, '.', '_' and '-'", k)
		}
		if k == "source" {
			return api.InvalidField(field, "the source Tag is set from source; don't give it as a Tag")
		}
		if !singleLine(tags[k]) {
			return api.InvalidField(field, "Tag %q needs a non-empty, single-line value", k)
		}
	}
	return nil
}

func singleLine(s string) bool { return strings.TrimSpace(s) != "" && !strings.ContainsAny(s, "\r\n") }

// alreadyImported refuses an Import if any Spec or Ticket in the Project
// already has a source Tag in the payload, listing every match.
func alreadyImported(r store.Reader, key string, p api.ImportPayload) error {
	var sources []string
	for _, s := range p.Specs {
		sources = append(sources, s.Source)
	}
	for _, t := range p.Tickets {
		sources = append(sources, t.Source)
	}
	if len(sources) == 0 {
		return nil
	}
	tag := &store.TagFilter{Key: "source", Values: sources}
	existing := map[string]int{}
	for _, s := range r.FindSpecs(key, store.SpecFilter{Tag: tag}) {
		existing[s.Tags["source"]] = s.Number
	}
	for _, t := range r.FindTickets(key, store.TicketFilter{Tag: tag}) {
		existing[t.Tags["source"]] = t.Number
	}
	var field string
	var matches []string
	check := func(f, src string) {
		if n, ok := existing[src]; ok {
			if field == "" {
				field = f
			}
			matches = append(matches, fmt.Sprintf("%s (%s)", src, domain.DisplayID(key, n)))
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
	return api.InvalidField(field, "this Project already has %d of these sources, so this Import has run before; nothing was imported: %s",
		len(matches), strings.Join(matches, ", "))
}

// importBranchTags checks each new Spec's branch Tag as set_tags checks
// one, and that no two of them share one name.
func importBranchTags(r store.Reader, key string, p api.ImportPayload) error {
	pr, _ := r.Project(key)
	first := map[string]string{} // name → the field that first gave it
	for i, s := range p.Specs {
		name, ok := s.Tags[domain.BranchTag]
		if !ok {
			continue
		}
		field := fmt.Sprintf("specs[%d].tags.%s", i, domain.BranchTag)
		if err := checkBranchTagName(pr, field, name); err != nil {
			return err
		}
		if err := checkBranchTagFree(r, key, field, 0, name, chooseAnotherName); err != nil {
			return err
		}
		if f, ok := first[name]; ok {
			return api.InvalidField(field, "branch %q is already given by %s: each Spec needs its own Spec Branch", name, f)
		}
		first[name] = field
	}
	return nil
}

// buildImport numbers every item from the counter in source order (each
// Spec, then its Tickets) and builds the records and the plan.
func (c *Core) buildImport(r store.Reader, key string, p api.ImportPayload) (store.Import, api.ImportResult) {
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
	res := api.ImportResult{StateCounts: map[domain.State]int{}}
	bySpec := map[int]int{} // Spec number → index in res.Specs
	for _, s := range p.Specs {
		sp := domain.Spec{Number: numbers[s.Source], Title: s.Title, Labels: s.Labels, Tags: withSource(s.Tags, s.Source),
			Created: now, Updated: now, Body: s.Body}
		im.Specs = append(im.Specs, sp)
		bySpec[sp.Number] = len(res.Specs)
		res.Specs = append(res.Specs, api.ImportedSpec{Source: s.Source, ID: domain.DisplayID(key, sp.Number), Title: sp.Title,
			Labels: orEmpty(sp.Labels), Tags: sp.Tags, Tickets: []api.ImportedTicket{}})
	}
	for _, i := range order {
		t := p.Tickets[i]
		state := t.State
		if state == "" {
			state = domain.Draft
		}
		tk := domain.Ticket{
			Number: numbers[t.Source], Title: t.Title, State: state, Spec: numbers[t.Spec],
			Labels: t.Labels, Tags: withSource(t.Tags, t.Source), BlockedBy: refs(t.BlockedBy), Replacements: refs(t.Replacements),
			Created: now, Updated: now, Description: t.Description,
			Moves: []domain.Move{{To: state, Actor: string(api.Agent), Time: now, Via: "import"}},
		}
		if t.Comment != "" {
			addComment(&tk, api.Agent, now, 0, t.Comment)
		}
		im.Tickets = append(im.Tickets, tk)
		res.StateCounts[state]++
		g := &res.Specs[bySpec[tk.Spec]]
		g.Tickets = append(g.Tickets, api.ImportedTicket{
			Source: t.Source, ID: domain.DisplayID(key, tk.Number), Title: tk.Title, State: state,
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
	ov := importOverlay{Reader: r, key: key, tickets: map[int]domain.Ticket{}}
	for _, t := range im.Tickets {
		ov.tickets[t.Number] = t
	}
	for _, t := range im.Tickets {
		self := func(n int) bool { return n == t.Number }
		if path := waitPath(ov, key, waitsOn(ov, key, t.Number), self); path != nil {
			return cycleError(key, append([]int{t.Number}, path...))
		}
		if path := replacementPath(ov, key, t.Replacements, t.Number, map[int]bool{}); path != nil {
			return &api.Error{Code: api.Cycle, Message: "the Replacements form a loop: " + strings.Join(displayIDs(key, append([]int{t.Number}, path...)), " → ") + " (each is replaced by the next)"}
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
		if t, ok := r.Ticket(key, n); ok && t.State == domain.Cancelled {
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
	tickets map[int]domain.Ticket
}

func (o importOverlay) Ticket(project string, n int, parts ...store.Part) (domain.Ticket, bool) {
	if t, ok := o.tickets[n]; ok && project == o.key {
		return t, true
	}
	return o.Reader.Ticket(project, n, parts...)
}
