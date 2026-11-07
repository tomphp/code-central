package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
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
}

// migrations is the real chain: one step from each older format. To bump
// Format, raise currentFormat in format.go and add the step from the old
// number here.
var migrations = []MigrationStep{}

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
	// Lock the staged store before it goes live, so the lock is never let go.
	lock, err := lockFile(stage)
	if err != nil {
		return err
	}
	if err := os.Rename(s.dir, bak); err != nil {
		unlock(lock)
		return err
	}
	done = true // from here a restart finishes the job (finishMigration)
	if stopMigrationBetweenRenames {
		unlock(lock)
		return errMigrationStopped
	}
	if err := os.Rename(stage, s.dir); err != nil {
		unlock(lock)
		if os.Rename(bak, s.dir) == nil {
			os.RemoveAll(stage)
		}
		return err
	}
	if err := syncDir(filepath.Dir(s.dir)); err != nil {
		unlock(lock)
		return err
	}
	unlock(s.lock) // the old store's lock, now in the backup
	s.lock = lock
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

// lockFile takes the exclusive lock on the store at dir.
func lockFile(dir string) (*os.File, error) {
	lock, err := os.OpenFile(filepath.Join(dir, "lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("%s: %w", dir, ErrLocked)
		}
		return nil, err
	}
	return lock, nil
}

func unlock(lock *os.File) {
	if lock != nil {
		syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
		lock.Close()
	}
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
