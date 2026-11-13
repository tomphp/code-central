package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Migrating the store (ADR 0003, design ticket 21). When store.yaml holds
// an older format, Open runs the chained steps from that format up to
// this build's, writing every file into a staging folder next to the
// store. Only once the whole staged store is on disk is the old store
// renamed to <store>.format-<n>.bak and the staging folder renamed into
// place. A failure before that deletes the staging folder and leaves the
// original store untouched. Backups are never deleted.
//
// Every schema change bumps Format, even an additive one: files are
// decoded strictly, so an older binary would otherwise fail on an
// "unknown field" instead of the clear "unknown format". core's schema
// snapshot test enforces this.

// MigrationStep migrates a store from format From to From+1.
type MigrationStep struct {
	From int
	// Name says what the step does; a failure names it.
	Name string
	// File rewrites one file, given its path relative to the store
	// (slash-separated, such as projects/acme/tickets/3.yaml). Nil copies
	// every file unchanged, which is all an additive change needs: the
	// migration itself rewrites the number in store.yaml.
	File func(rel string, data []byte) ([]byte, error)
	// After runs once the whole store is staged, given the staging
	// folder, for a change that spans files, such as one file's content
	// naming another. Nil does nothing.
	After func(stage string) error
}

// migrations is the real chain: one step from each older format. To bump
// Format, raise currentFormat in format.go and add the step from the old
// number here.
var migrations = []MigrationStep{
	{From: 1, Name: "add the typed link on Specs and Tickets, and its edit history keys"},
	{From: 2, Name: "archived becomes a time", File: archivedBecomesATime},
	{From: 3, Name: "add a Claim's working_dir and resume_command, and their copies on Failure Reports"},
	{From: 4, Name: "add the Accepted State, a finish's branch and commit, and a land's commit on its Move"},
	{From: 5, Name: "add the global Options file"},
	{From: 6, Name: "add a Project's Landing Method"},
	{From: 7, Name: "add a Project's CI settings"},
	{From: 8, Name: "add a Project's CI Rules"},
	{From: 9, Name: "add the gh path Option"},
	{From: 10, Name: "add a Ticket's pushed commits and a Project's CI Runs"},
	{From: 11, Name: "add the human's Land anyway on a pushed commit"},
	{From: 12, Name: "add the human's Re-run request on a pushed commit"},
	{From: 13, Name: "add a Project's GitHub Issues settings"},
	{From: 14, Name: "add a Project's Unblock on push"},
	{From: 15, Name: "add a Ticket's awaiting triage"},
	{From: 16, Name: "add the keys of the comments written for a Link"},
	{From: 17, Name: "add a Link's outbound flag"},
	{From: 18, Name: "add the model and effort a Claim's latest turn ran with"},
	{From: 19, Name: "add a Project's Azure Pipelines settings and the partially succeeded CI conclusion"},
	{From: 20, Name: "add a Project's Land per Spec"},
	{From: 21, Name: "add a Ticket's Spec merge"},
	{From: 22, Name: "add a Project's Pull Request Landing Method and PAT variable"},
	{From: 23, Name: "add a Ticket's pull request and a pushed commit's branch"},
	{From: 24, Name: "label each CI Rule's Spec and Issues Spec long-lived", After: labelLongLivedSpecs},
	{From: 25, Name: "add a Project's base"},
	{From: 26, Name: "add a Project's Stage Guides settings"},
	{From: 27, Name: "add a Project's CI responses, one per pushed commit"},
	{From: 28, Name: "add a Project's CI Fixes Spec and a CI response's fix Ticket"},
	{From: 29, Name: "add a Project's Model Assignment setting"},
	{From: 30, Name: "add a Project's Small Changes Spec"},
	{From: 31, Name: "add the Focus Pop-up Option"},
	{From: 32, Name: "add a Ticket's Spec merge mark"},
	{From: 33, Name: "add a spec Stage's Spec recommendation"},
	{From: 34, Name: "add a Project's Auto-accept kinds and a Move's auto-accept cause"},
}

// archivedBecomesATime is format 2 to 3 (ticket 39): project.yaml's
// archived flag becomes the time the Project was archived. Formats 1 and 2 had
// no way to archive, so a true flag is a hand edit with no time; it
// takes the Project's created time.
func archivedBecomesATime(rel string, data []byte) ([]byte, error) {
	if !strings.HasPrefix(rel, "projects/") || !strings.HasSuffix(rel, "/project.yaml") {
		return data, nil
	}
	var old struct {
		Key         string `yaml:"key"`
		DisplayName string `yaml:"display_name"`
		Created     string `yaml:"created"`
		Mode        string `yaml:"mode"`
		Archived    bool   `yaml:"archived,omitempty"`
	}
	if field, err := decodeStrict(data, &old); err != nil {
		return nil, fmt.Errorf("%s: %w", field, err)
	}
	pf := projectFile{Key: old.Key, DisplayName: old.DisplayName, Created: old.Created, Mode: old.Mode}
	if old.Archived {
		pf.Archived = old.Created
	}
	return encodeYAML(pf)
}

var (
	errMigrationStopped         = errors.New("store: migration stopped by test hook")
	stopMigrationBetweenRenames bool
)

// MigrationError is a migration step that failed on one file. The store
// is untouched.
type MigrationError struct {
	From, To int
	Step     string
	File     string
	Err      error
}

func (e *MigrationError) Error() string {
	return fmt.Sprintf("migrating the store from format %d to %d, step %q failed on %s: %v (the store is untouched)",
		e.From, e.To, e.Step, e.File, e.Err)
}

func (e *MigrationError) Unwrap() error { return e.Err }

// stagingFor is the staging folder a migration of dir writes into.
func stagingFor(dir string) string {
	return filepath.Join(filepath.Dir(dir), "."+filepath.Base(dir)+".migrating")
}

// backupFor is where dir is kept after migrating from format n.
func backupFor(dir string, n int) string {
	return filepath.Join(filepath.Dir(dir), fmt.Sprintf("%s.format-%d.bak", filepath.Base(dir), n))
}

// finishMigration runs before the store is created or locked: if a
// migration stopped between its two renames, the store folder is gone
// and its complete staging folder (store.yaml is written last) is put
// in place.
func finishMigration(dir string) error {
	if _, err := os.Stat(dir); !errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	stage := stagingFor(dir)
	if _, err := os.Stat(filepath.Join(stage, "store.yaml")); err != nil {
		return nil
	}
	if err := os.Rename(stage, dir); err != nil {
		return err
	}
	return syncDir(filepath.Dir(dir))
}

// migrate brings the store up to s.format, if store.yaml holds an older
// one. It runs after Import recovery and before load.
func (s *FileStore) migrate() error {
	from, ok, err := s.readFormat()
	if err != nil || !ok || from >= s.format {
		return err // a newer format is refused by load
	}
	if from < 1 {
		return fmt.Errorf("store.yaml: unknown format %d (this code-central understands format %d)", from, s.format)
	}
	var chain []MigrationStep
	for v := from; v < s.format; v++ {
		i := -1
		for j, st := range s.steps {
			if st.From == v {
				i = j
			}
		}
		if i < 0 {
			return fmt.Errorf("store.yaml: format %d: this code-central has no migration from format %d", from, v)
		}
		chain = append(chain, s.steps[i])
	}
	bak := backupFor(s.dir, from)
	if _, err := os.Lstat(bak); !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("cannot migrate the store from format %d: the backup %s already exists; move it aside and start again (the store is untouched)", from, bak)
	}

	stage := stagingFor(s.dir)
	if err := os.RemoveAll(stage); err != nil { // left by a migration that never finished
		return err
	}
	done := false
	defer func() {
		if !done {
			os.RemoveAll(stage)
		}
	}()
	if err := s.stage(stage, from, chain); err != nil {
		return err
	}
	if err := atomicWrite(filepath.Join(stage, "store.yaml"), fmt.Appendf(nil, "format: %d\n", s.format)); err != nil {
		return err
	}
	// Lock the staged store before it goes live, so the lock is never let
	// go (on Windows the store's own lock already covers it).
	lock, err := lockStaged(stage, s.dir)
	if err != nil {
		return err
	}
	if err := os.Rename(s.dir, bak); err != nil {
		lock.release()
		return err
	}
	done = true // from here a restart finishes the job (finishMigration)
	if stopMigrationBetweenRenames {
		lock.release()
		return errMigrationStopped
	}
	if err := os.Rename(stage, s.dir); err != nil {
		lock.release()
		if os.Rename(bak, s.dir) == nil {
			os.RemoveAll(stage)
		}
		return err
	}
	if err := syncDir(filepath.Dir(s.dir)); err != nil {
		lock.release()
		return err
	}
	if lock != nil { // nil where the store's own lock already covers the staged store
		s.lock.release() // the old store's lock, now in the backup
		s.lock = lock
	}
	s.migrated = &Migration{From: from, To: s.format, Backup: bak}
	return nil
}

// readFormat reads store.yaml's format; ok is false when there is none.
func (s *FileStore) readFormat() (format int, ok bool, err error) {
	data, err := os.ReadFile(filepath.Join(s.dir, "store.yaml"))
	if errors.Is(err, fs.ErrNotExist) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	var f storeFile
	if field, err := decodeStrict(data, &f); err != nil {
		return 0, false, &LoadError{File: "store.yaml", Field: field, Msg: err.Error()}
	}
	return f.Format, true, nil
}

// stage writes every file of the store, run through chain, into stage,
// and fsyncs them. The lock, store.yaml, the Import staging folder and
// leftover temp files are not copied.
func (s *FileStore) stage(stage string, from int, chain []MigrationStep) error {
	var rels []string
	dirs := map[string]bool{stage: true}
	err := filepath.WalkDir(s.dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(s.dir, p)
		rel = filepath.ToSlash(rel)
		switch {
		case rel == ".":
			return nil
		case rel == "lock" || rel == "store.yaml":
			return nil
		case rel == stagingDir && d.IsDir():
			return filepath.SkipDir
		case strings.HasPrefix(d.Name(), "."):
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		case d.IsDir():
			dirs[filepath.Join(stage, filepath.FromSlash(rel))] = true
			return nil
		case !d.Type().IsRegular():
			return &MigrationError{From: from, To: s.format, Step: "copy", File: rel, Err: errors.New("not a regular file")}
		}
		rels = append(rels, rel)
		return nil
	})
	if err != nil {
		return err
	}
	for d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}

	// Files are independent, so they're rewritten and fsynced in
	// parallel: a restart has to stay well inside Claude Code's ~31s.
	errs := make([]error, len(rels))
	parallel(len(rels), func(i int) { errs[i] = s.stageFile(stage, rels[i], from, chain) })
	for _, err := range errs { // the first failing file, in walk order
		if err != nil {
			return err
		}
	}
	for _, st := range chain {
		if st.After == nil {
			continue
		}
		if err := st.After(stage); err != nil {
			return &MigrationError{From: st.From, To: st.From + 1, Step: st.Name, File: "the store", Err: err}
		}
	}
	for d := range dirs {
		if err := syncDir(d); err != nil {
			return err
		}
	}
	return nil
}

func (s *FileStore) stageFile(stage, rel string, from int, chain []MigrationStep) error {
	data, err := os.ReadFile(filepath.Join(s.dir, filepath.FromSlash(rel)))
	if err != nil {
		return &MigrationError{From: from, To: s.format, Step: "read", File: rel, Err: err}
	}
	for _, st := range chain {
		if st.File == nil {
			continue
		}
		if data, err = st.File(rel, data); err != nil {
			return &MigrationError{From: st.From, To: st.From + 1, Step: st.Name, File: rel, Err: err}
		}
	}
	if err := writeFlushed(filepath.Join(stage, filepath.FromSlash(rel)), data); err != nil {
		return &MigrationError{From: from, To: s.format, Step: "write", File: rel, Err: err}
	}
	return nil
}

// Migration is a migration Open ran: the store went from format From to
// To, and the old store is kept at Backup.
type Migration struct {
	From, To int
	Backup   string
}

// Migrated is the migration Open ran, or nil if the store was already at
// this build's format.
func (s *FileStore) Migrated() *Migration { return s.migrated }
