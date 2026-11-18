//go:build !js

package store

import (
	"fmt"

	"github.com/tomoram/code-central/domain"
)

// sqlWriter writes one record in the Update's transaction, keeping the
// side tables with it.
type sqlWriter struct {
	sqlReader
	wrote bool
}

// exec runs a statement with the dialect's placeholders.
func (w *sqlWriter) exec(q string, args ...any) error {
	_, err := w.q.ExecContext(w.ctx(), w.d.rebind(q), args...)
	return err
}

// begin checks the one-write rule and, for a record of a Project, that
// the Project exists.
func (w *sqlWriter) begin(project string, ofProject bool) error {
	if w.wrote {
		return ErrSecondWrite
	}
	if ofProject && !w.hasProject(project) {
		return ErrUnknownProject
	}
	return nil
}

func (w *sqlWriter) PutProject(p domain.Project) error {
	if err := w.begin(p.Key, false); err != nil {
		return err
	}
	doc, err := jsonDoc(projectFileOf(p))
	if err != nil {
		return err
	}
	if err := w.exec(`INSERT INTO projects (key, doc) VALUES (?, ?) ON CONFLICT (key) DO UPDATE SET doc = excluded.doc`, p.Key, string(doc)); err != nil {
		return err
	}
	w.wrote = true
	return nil
}

func (w *sqlWriter) PutSpec(project string, s domain.Spec) error {
	if err := w.begin(project, true); err != nil {
		return err
	}
	if err := w.writeSpec(project, s); err != nil {
		return err
	}
	w.wrote = true
	return nil
}

// writeSpec writes s whole, refusing one its document couldn't give
// back: a File Store would refuse the same record at its next open, and
// a row no read can decode would fail every later read.
func (w *sqlWriter) writeSpec(project string, s domain.Spec) error {
	doc, err := specDoc(s)
	if err != nil {
		return err
	}
	if _, err := specFromDoc(doc, s.Number); err != nil {
		return fmt.Errorf("%s-%d: %w", project, s.Number, err)
	}
	if err := w.exec(`INSERT INTO specs (project, number, created, updated, link_integration, link_id, doc) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (project, number) DO UPDATE SET created = excluded.created, updated = excluded.updated,
		link_integration = excluded.link_integration, link_id = excluded.link_id, doc = excluded.doc`,
		project, s.Number, unixNanos(s.Created), unixNanos(s.Updated), s.Link.Integration, s.Link.ID, string(doc)); err != nil {
		return err
	}
	return w.writeMarks(project, "spec", s.Number, s.Labels, s.Tags)
}

// writeMarks replaces the record's labels and tags rows.
func (w *sqlWriter) writeMarks(project, kind string, n int, labels []string, tags map[string]string) error {
	if err := w.exec(`DELETE FROM labels WHERE project = ? AND kind = ? AND number = ?`, project, kind, n); err != nil {
		return err
	}
	if err := w.exec(`DELETE FROM tags WHERE project = ? AND kind = ? AND number = ?`, project, kind, n); err != nil {
		return err
	}
	for _, l := range labels {
		if err := w.exec(`INSERT INTO labels (project, kind, number, label) VALUES (?, ?, ?, ?)`, project, kind, n, l); err != nil {
			return err
		}
	}
	for k, v := range tags {
		if err := w.exec(`INSERT INTO tags (project, kind, number, key, value) VALUES (?, ?, ?, ?, ?)`, project, kind, n, k, v); err != nil {
			return err
		}
	}
	return nil
}

func (w *sqlWriter) PutTicket(project string, t domain.Ticket, parts ...Part) error {
	if err := w.begin(project, true); err != nil {
		return err
	}
	stored, _ := w.Ticket(project, t.Number, History...)
	if w.err != nil && *w.err != nil {
		return *w.err
	}
	if err := w.writeTicket(project, withHead(withParts(t, stored, parts))); err != nil {
		return err
	}
	w.wrote = true
	return nil
}

// writeTicket writes t whole, History and derived head included, and
// its side-table rows. It refuses a Ticket its documents couldn't give
// back, as writeSpec does.
func (w *sqlWriter) writeTicket(project string, t domain.Ticket) error {
	node, err := docNode(ticketFileOf(t))
	if err != nil {
		return err
	}
	row := ticketRow{parts: make([][]byte, 0, len(History))}
	for _, p := range History {
		doc, err := jsonOf(takeKey(node, string(p)))
		if err != nil {
			return err
		}
		row.parts = append(row.parts, doc)
	}
	if row.doc, err = jsonOf(node); err != nil {
		return err
	}
	if row.head, err = jsonDoc(headFileOf(t.Head)); err != nil {
		return err
	}
	if _, err := row.ticket(t.Number, History); err != nil {
		return fmt.Errorf("%s-%d: %w", project, t.Number, err)
	}
	args := []any{project, t.Number, string(t.State), t.Spec, unixNanos(t.Created), unixNanos(t.Updated), t.Link.Integration, t.Link.ID, string(row.doc), string(row.head)}
	for _, p := range row.parts {
		args = append(args, string(p))
	}
	if err := w.exec(`INSERT INTO tickets (project, number, state, spec, created, updated, link_integration, link_id, doc, head, claims, rounds, comments, moves, edits, failure_reports)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (project, number) DO UPDATE SET state = excluded.state, spec = excluded.spec, created = excluded.created, updated = excluded.updated,
		link_integration = excluded.link_integration, link_id = excluded.link_id, doc = excluded.doc, head = excluded.head, claims = excluded.claims,
		rounds = excluded.rounds, comments = excluded.comments, moves = excluded.moves, edits = excluded.edits, failure_reports = excluded.failure_reports`, args...); err != nil {
		return err
	}
	if err := w.writeMarks(project, "ticket", t.Number, t.Labels, t.Tags); err != nil {
		return err
	}
	for _, table := range []string{"pushed_commits", "blockers", "replacements", "claims"} {
		if err := w.exec(`DELETE FROM `+table+` WHERE project = ? AND number = ?`, project, t.Number); err != nil {
			return err
		}
	}
	for i, pc := range t.PushedCommits {
		if err := w.exec(`INSERT INTO pushed_commits (project, number, seq, repository, sha, branch) VALUES (?, ?, ?, ?, ?, ?)`, project, t.Number, i, pc.Repository, pc.Commit, pc.Branch); err != nil {
			return err
		}
	}
	for _, b := range t.BlockedBy {
		if err := w.exec(`INSERT INTO blockers (project, number, blocker) VALUES (?, ?, ?)`, project, t.Number, b); err != nil {
			return err
		}
	}
	for _, rp := range t.Replacements {
		if err := w.exec(`INSERT INTO replacements (project, number, replacement) VALUES (?, ?, ?)`, project, t.Number, rp); err != nil {
			return err
		}
	}
	for i, c := range t.Claims {
		if err := w.exec(`INSERT INTO claims (project, number, idx, session_id, started) VALUES (?, ?, ?, ?, ?)`, project, t.Number, i, c.SessionID, unixNanos(c.Started)); err != nil {
			return err
		}
	}
	return nil
}

func (w *sqlWriter) PutImport(project string, im Import) error {
	if err := w.begin(project, true); err != nil {
		return err
	}
	specs, tickets := w.numbers(project)
	if w.err != nil && *w.err != nil {
		return *w.err
	}
	hasSpec := func(n int) bool { return specs[n] }
	hasTicket := func(n int) bool { return tickets[n] }
	if err := checkImport(project, im, hasSpec, hasTicket); err != nil {
		return err
	}
	for _, s := range im.Specs {
		if err := w.writeSpec(project, s); err != nil {
			return err
		}
	}
	for _, sp := range im.RewrittenSpecs {
		if err := w.writeSpec(project, sp); err != nil {
			return err
		}
	}
	for _, t := range im.Tickets {
		if err := w.writeTicket(project, withHead(t)); err != nil {
			return err
		}
	}
	for _, t := range im.Rewritten {
		if err := w.writeTicket(project, withHead(t)); err != nil {
			return err
		}
	}
	w.wrote = true
	return nil
}

// numbers is the project's Spec numbers and Ticket numbers in use.
func (w *sqlWriter) numbers(project string) (specs, tickets map[int]bool) {
	specs, tickets = map[int]bool{}, map[int]bool{}
	for table, set := range map[string]map[int]bool{"specs": specs, "tickets": tickets} {
		rows, err := w.query(`SELECT number FROM `+table+` WHERE project = ?`, project)
		if err != nil {
			w.fail(err)
			return
		}
		for rows.Next() {
			var n int
			if err := rows.Scan(&n); err != nil {
				w.fail(err)
				rows.Close()
				return
			}
			set[n] = true
		}
		w.fail(rows.Err())
		rows.Close()
	}
	return specs, tickets
}

func (w *sqlWriter) PutOptions(o domain.Options) error {
	if err := checkOptions(o); err != nil {
		return err
	}
	if err := w.begin("", false); err != nil {
		return err
	}
	doc, err := jsonDoc(optionsFileOf(o))
	if err != nil {
		return err
	}
	if err := w.exec(`INSERT INTO options (id, doc) VALUES (1, ?) ON CONFLICT (id) DO UPDATE SET doc = excluded.doc`, string(doc)); err != nil {
		return err
	}
	w.wrote = true
	return nil
}

// Announce is ignored: no other service shares a SQLite Store, and a
// Postgres Store's notices come with Ticket code-central-1048.
func (w *sqlWriter) Announce(Change) {}

func (w *sqlWriter) PutCIRuns(project string, runs []domain.CIRun) error {
	if err := w.begin(project, true); err != nil {
		return err
	}
	doc, err := jsonDoc(ciRunsFileOf(domain.CIRuns{Runs: runs}))
	if err != nil {
		return err
	}
	if err := w.exec(`INSERT INTO ci_runs (project, doc) VALUES (?, ?) ON CONFLICT (project) DO UPDATE SET doc = excluded.doc`, project, string(doc)); err != nil {
		return err
	}
	w.wrote = true
	return nil
}
