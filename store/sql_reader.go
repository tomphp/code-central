//go:build !js

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"iter"
	"slices"
	"strings"
	"time"

	"github.com/tomoram/code-central/domain"
)

// sqlReader answers the Reader's queries from the database over q. The
// Reader's reads report nothing, so a statement or document that fails
// is kept in err, which View and Update return once fn is done.
type sqlReader struct {
	q   querier
	d   dialect
	err *error
}

func (r sqlReader) ctx() context.Context { return context.Background() }

// fail keeps the first failure for View or Update to return.
func (r sqlReader) fail(err error) {
	if err != nil && r.err != nil && *r.err == nil {
		*r.err = err
	}
}

// query runs a SELECT with the dialect's placeholders.
func (r sqlReader) query(q string, args ...any) (*sql.Rows, error) {
	return r.q.QueryContext(r.ctx(), r.d.rebind(q), args...)
}

// row runs a SELECT of one row with the dialect's placeholders.
func (r sqlReader) row(q string, args ...any) *sql.Row {
	return r.q.QueryRowContext(r.ctx(), r.d.rebind(q), args...)
}

// scanDoc reads one document column, found false when the row isn't
// there.
func (r sqlReader) scanDoc(q string, args ...any) (doc []byte, found bool) {
	err := r.row(q, args...).Scan(&doc)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false
	}
	if err != nil {
		r.fail(err)
		return nil, false
	}
	return doc, true
}

// The records' documents, each decoded strictly as its file is.

func projectFromDoc(doc []byte) (domain.Project, error) {
	var f projectFile
	if field, err := fromJSONDoc(doc, &f); err != nil {
		return domain.Project{}, fmt.Errorf("project document: field %s: %w", field, err)
	}
	p, lerr := projectOf(f)
	if lerr != nil {
		return domain.Project{}, fmt.Errorf("project document: %w", lerr)
	}
	p.Landing = landingOrDefault(p.Landing)
	return p, nil
}

// specDoc is s as its document: its frontmatter's keys, then body.
func specDoc(s domain.Spec) ([]byte, error) {
	n, err := docNode(specFrontOf(s))
	if err != nil {
		return nil, err
	}
	putKey(n, "body", stringNode(s.Body))
	return jsonOf(n)
}

func specFromDoc(doc []byte, n int) (domain.Spec, error) {
	node, err := jsonNode(doc)
	if err != nil {
		return domain.Spec{}, fmt.Errorf("spec document: %w", err)
	}
	body := takeKey(node, "body")
	if body == nil {
		return domain.Spec{}, errors.New("spec document: field body: missing")
	}
	var f specFront
	if field, err := decodeDocNode(node, &f); err != nil {
		return domain.Spec{}, fmt.Errorf("spec document: field %s: %w", field, err)
	}
	s, lerr := specOf(f, body.Value, n)
	if lerr != nil {
		return domain.Spec{}, fmt.Errorf("spec document: %w", lerr)
	}
	return s, nil
}

// ticketColumns is the Ticket row's columns a read with parts selects:
// its document, its derived head, then a column per part named.
func ticketColumns(parts []Part) string {
	cols := []string{"doc", "head"}
	for _, p := range History {
		if slices.Contains(parts, p) {
			cols = append(cols, string(p))
		}
	}
	return strings.Join(cols, ", ")
}

// ticketRow is one Ticket row's columns, as ticketColumns selects them.
type ticketRow struct {
	doc, head []byte
	parts     [][]byte
}

// scan reads the row's columns after any leading ones in first.
func (tr *ticketRow) scan(rows interface{ Scan(...any) error }, parts []Part, first ...any) error {
	n := 0
	for _, p := range History {
		if slices.Contains(parts, p) {
			n++
		}
	}
	tr.parts = make([][]byte, n)
	dest := append(first, &tr.doc, &tr.head)
	for i := range tr.parts {
		dest = append(dest, &tr.parts[i])
	}
	return rows.Scan(dest...)
}

// ticket is the Ticket the row holds, with the parts read.
func (tr ticketRow) ticket(n int, parts []Part) (domain.Ticket, error) {
	node, err := jsonNode(tr.doc)
	if err != nil {
		return domain.Ticket{}, fmt.Errorf("ticket document: %w", err)
	}
	i := 0
	for _, p := range History {
		if !slices.Contains(parts, p) {
			continue
		}
		pn, err := jsonNode(tr.parts[i])
		if err != nil {
			return domain.Ticket{}, fmt.Errorf("ticket %s document: %w", p, err)
		}
		putKey(node, string(p), pn)
		i++
	}
	var f ticketFile
	if field, err := decodeDocNode(node, &f); err != nil {
		return domain.Ticket{}, fmt.Errorf("ticket document: field %s: %w", field, err)
	}
	t, lerr := ticketOf(f, n)
	if lerr != nil {
		return domain.Ticket{}, fmt.Errorf("ticket document: %w", lerr)
	}
	var hf headFile
	if field, err := fromJSONDoc(tr.head, &hf); err != nil {
		return domain.Ticket{}, fmt.Errorf("ticket head document: field %s: %w", field, err)
	}
	if t.Head, lerr = headOf(hf); lerr != nil {
		return domain.Ticket{}, fmt.Errorf("ticket head document: %w", lerr)
	}
	return t, nil
}

func (r sqlReader) Project(key string) (domain.Project, bool) {
	doc, ok := r.scanDoc(`SELECT doc FROM projects WHERE key = ?`, key)
	if !ok {
		return domain.Project{}, false
	}
	p, err := projectFromDoc(doc)
	if err != nil {
		r.fail(err)
		return domain.Project{}, false
	}
	return p, true
}

func (r sqlReader) Projects() []domain.Project {
	out := []domain.Project{}
	rows, err := r.query(`SELECT doc FROM projects ORDER BY key`)
	if err != nil {
		r.fail(err)
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var doc []byte
		if err := rows.Scan(&doc); err != nil {
			r.fail(err)
			return out
		}
		p, err := projectFromDoc(doc)
		if err != nil {
			r.fail(err)
			return out
		}
		out = append(out, p)
	}
	r.fail(rows.Err())
	return out
}

// hasProject is whether the project exists.
func (r sqlReader) hasProject(key string) bool {
	var one int
	err := r.row(`SELECT 1 FROM projects WHERE key = ?`, key).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false
	}
	if err != nil {
		r.fail(err)
		return false
	}
	return true
}

func (r sqlReader) Spec(project string, n int) (domain.Spec, bool) {
	doc, ok := r.scanDoc(`SELECT doc FROM specs WHERE project = ? AND number = ?`, project, n)
	if !ok {
		return domain.Spec{}, false
	}
	s, err := specFromDoc(doc, n)
	if err != nil {
		r.fail(err)
		return domain.Spec{}, false
	}
	return s, true
}

func (r sqlReader) Ticket(project string, n int, parts ...Part) (domain.Ticket, bool) {
	row := r.row(`SELECT `+ticketColumns(parts)+` FROM tickets WHERE project = ? AND number = ?`, project, n)
	var tr ticketRow
	err := tr.scan(row, parts)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Ticket{}, false
	}
	if err != nil {
		r.fail(err)
		return domain.Ticket{}, false
	}
	t, err := tr.ticket(n, parts)
	if err != nil {
		r.fail(err)
		return domain.Ticket{}, false
	}
	return t, true
}

// marks adds the conditions label, tag and link put on the records of
// kind at alias, as picksMarks tests them.
func marks(label string, tag *TagFilter, link *LinkFilter, kind, alias string, conds *[]string, args *[]any) {
	if label != "" {
		*conds = append(*conds, `EXISTS (SELECT 1 FROM labels l WHERE l.project = `+alias+`.project AND l.kind = ? AND l.number = `+alias+`.number AND l.label = ?)`)
		*args = append(*args, kind, label)
	}
	if tag != nil {
		cond := `EXISTS (SELECT 1 FROM tags g WHERE g.project = ` + alias + `.project AND g.kind = ? AND g.number = ` + alias + `.number AND g.key = ?`
		*args = append(*args, kind, tag.Key)
		if len(tag.Values) > 0 {
			cond += ` AND g.value IN (` + placeholders(len(tag.Values)) + `)`
			for _, v := range tag.Values {
				*args = append(*args, v)
			}
		}
		*conds = append(*conds, cond+`)`)
	}
	if link != nil {
		*conds = append(*conds, alias+`.link_integration = ?`)
		*args = append(*args, link.Integration)
		if link.ID != "" {
			*conds = append(*conds, alias+`.link_id = ?`)
			*args = append(*args, link.ID)
		}
	}
}

// where is f's conditions on the tickets at alias t, and their arguments.
func (f TicketFilter) where(project string) (string, []any) {
	conds := []string{`t.project = ?`}
	args := []any{project}
	in := func(col string, vs []any) {
		conds = append(conds, col+` IN (`+placeholders(len(vs))+`)`)
		args = append(args, vs...)
	}
	if len(f.Spec) > 0 {
		vs := make([]any, len(f.Spec))
		for i, n := range f.Spec {
			vs[i] = n
		}
		in(`t.spec`, vs)
	}
	if len(f.States) > 0 {
		vs := make([]any, len(f.States))
		for i, s := range f.States {
			vs[i] = string(s)
		}
		in(`t.state`, vs)
	}
	marks(f.Label, f.Tag, f.Link, "ticket", "t", &conds, &args)
	const pushed = `EXISTS (SELECT 1 FROM pushed_commits pc WHERE pc.project = t.project AND pc.number = t.number`
	if f.PushedCommit != nil {
		conds = append(conds, pushed+` AND pc.repository = ? AND pc.sha = ?)`)
		args = append(args, f.PushedCommit.Repository, f.PushedCommit.Commit)
	}
	if f.PushedBranch != nil {
		conds = append(conds, pushed+` AND pc.repository = ? AND pc.branch = ?)`)
		args = append(args, f.PushedBranch.Repository, f.PushedBranch.Branch)
	}
	if f.HasPushes {
		conds = append(conds, pushed+`)`)
	}
	if f.BlockedBy != 0 {
		conds = append(conds, `EXISTS (SELECT 1 FROM blockers b WHERE b.project = t.project AND b.number = t.number AND b.blocker = ?)`)
		args = append(args, f.BlockedBy)
	}
	if f.ReplacedBy != 0 {
		conds = append(conds, `EXISTS (SELECT 1 FROM replacements rp WHERE rp.project = t.project AND rp.number = t.number AND rp.replacement = ?)`)
		args = append(args, f.ReplacedBy)
	}
	return strings.Join(conds, " AND "), args
}

// orderBy is the ORDER BY clause of o on the tickets at alias t.
func (o Order) orderBy() string {
	if o == ByClaim {
		return `ORDER BY t.created, t.number`
	}
	return `ORDER BY t.number`
}

func (r sqlReader) FindTickets(project string, f TicketFilter) []domain.Ticket {
	out := []domain.Ticket{}
	where, args := f.where(project)
	rows, err := r.query(`SELECT t.number, `+ticketColumns(f.Parts)+` FROM tickets t WHERE `+where+` `+f.Order.orderBy(), args...)
	if err != nil {
		r.fail(err)
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var n int
		var tr ticketRow
		if err := tr.scan(rows, f.Parts, &n); err != nil {
			r.fail(err)
			return out
		}
		t, err := tr.ticket(n, f.Parts)
		if err != nil {
			r.fail(err)
			return out
		}
		out = append(out, t)
	}
	r.fail(rows.Err())
	return out
}

func (r sqlReader) FindSpecs(project string, f SpecFilter) []domain.Spec {
	out := []domain.Spec{}
	conds := []string{`s.project = ?`}
	args := []any{project}
	marks(f.Label, f.Tag, f.Link, "spec", "s", &conds, &args)
	rows, err := r.query(`SELECT s.number, s.doc FROM specs s WHERE `+strings.Join(conds, " AND ")+` ORDER BY s.number`, args...)
	if err != nil {
		r.fail(err)
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var n int
		var doc []byte
		if err := rows.Scan(&n, &doc); err != nil {
			r.fail(err)
			return out
		}
		s, err := specFromDoc(doc, n)
		if err != nil {
			r.fail(err)
			return out
		}
		out = append(out, s)
	}
	r.fail(rows.Err())
	return out
}

func (r sqlReader) StateCounts(project string) map[domain.State]int {
	out := map[domain.State]int{}
	rows, err := r.query(`SELECT state, COUNT(*) FROM tickets WHERE project = ? GROUP BY state`, project)
	if err != nil {
		r.fail(err)
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var state string
		var n int
		if err := rows.Scan(&state, &n); err != nil {
			r.fail(err)
			return out
		}
		out[domain.State(state)] = n
	}
	r.fail(rows.Err())
	return out
}

func (r sqlReader) ClaimBySession(sessionID string) (SessionClaim, bool) {
	if sessionID == "" {
		return SessionClaim{}, false
	}
	var project string
	var n, idx int
	err := r.row(`SELECT project, number, idx FROM claims WHERE session_id = ? ORDER BY started DESC, project, number, idx LIMIT 1`, sessionID).Scan(&project, &n, &idx)
	if errors.Is(err, sql.ErrNoRows) {
		return SessionClaim{}, false
	}
	if err != nil {
		r.fail(err)
		return SessionClaim{}, false
	}
	t, ok := r.Ticket(project, n, Claims)
	if !ok || idx >= len(t.Claims) {
		r.fail(fmt.Errorf("store: claim %d of %s-%d, found by session, is missing", idx, project, n))
		return SessionClaim{}, false
	}
	return SessionClaim{Project: project, Ticket: n, Claim: t.Claims[idx]}, true
}

func (r sqlReader) TicketCommits(project string) []TicketCommit {
	out := []TicketCommit{}
	for _, t := range r.FindTickets(project, TicketFilter{Parts: []Part{Moves}}) {
		out = append(out, ticketCommitsOf(project, t)...)
	}
	return out
}

func (r sqlReader) Records(project string) iter.Seq[Record] {
	return func(yield func(Record) bool) {
		for _, s := range r.FindSpecs(project, SpecFilter{}) {
			if !yield(Record{Spec: &s}) {
				return
			}
		}
		for _, t := range r.FindTickets(project, TicketFilter{Parts: History}) {
			if !yield(Record{Ticket: &t}) {
				return
			}
		}
	}
}

func (r sqlReader) NextNumber(project string) int {
	var high int
	err := r.row(`SELECT COALESCE(MAX(n), 0) FROM (SELECT number AS n FROM specs WHERE project = ? UNION ALL SELECT number FROM tickets WHERE project = ?) AS numbers`, project, project).Scan(&high)
	if err != nil {
		r.fail(err)
	}
	return high + 1
}

func (r sqlReader) Options() domain.Options {
	doc, ok := r.scanDoc(`SELECT doc FROM options WHERE id = 1`)
	if !ok {
		return domain.DefaultOptions()
	}
	var f optionsFile
	if field, err := fromJSONDoc(doc, &f); err != nil {
		r.fail(fmt.Errorf("options document: field %s: %w", field, err))
		return domain.DefaultOptions()
	}
	o, lerr := optionsOf(f)
	if lerr != nil {
		r.fail(fmt.Errorf("options document: %w", lerr))
		return domain.DefaultOptions()
	}
	return o
}

func (r sqlReader) CIRuns(project string) []domain.CIRun {
	doc, ok := r.scanDoc(`SELECT doc FROM ci_runs WHERE project = ?`, project)
	if !ok {
		if r.hasProject(project) {
			return []domain.CIRun{}
		}
		return nil
	}
	var f ciRunsFile
	if field, err := fromJSONDoc(doc, &f); err != nil {
		r.fail(fmt.Errorf("ci runs document: field %s: %w", field, err))
		return nil
	}
	runs, lerr := ciRunsOf(f)
	if lerr != nil {
		r.fail(fmt.Errorf("ci runs document: %w", lerr))
		return nil
	}
	if runs == nil {
		runs = []domain.CIRun{}
	}
	return runs
}

// unixNanos is t as the integer the indexed time columns hold, which
// orders as the times do; the zero time is 0.
func unixNanos(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixNano()
}
