package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
)

// Import is the one bulk write (ADR 0003), used by an Import and by a
// submitted Requirement. Under the write lock, every
// new file is written and fsynced into staging/<import-id>/; then a
// manifest naming each file's final path is written and fsynced; then
// each file is renamed into place; then the staging folder, manifest and
// all, is deleted. Memory changes after that. At startup a manifest left
// behind is rolled forward, and a staging folder with no manifest is
// deleted, so a crash at any point leaves all of the Import or none.

const (
	stagingDir   = "staging"
	manifestName = "manifest.yaml"
)

var errImportStopped = errors.New("store: import stopped by test hook")

type manifestFile struct {
	Files []manifestEntry `yaml:"files"`
}

// manifestEntry is one staged file and where it goes, relative to the
// store directory.
type manifestEntry struct {
	Staged string `yaml:"staged"`
	Path   string `yaml:"path"`
}

func (s *FileStore) writeImport(project string, im Import) error {
	var records []any
	for _, sp := range im.Specs {
		records = append(records, sp)
	}
	for _, t := range im.Tickets {
		records = append(records, t)
	}
	step := 0
	stop := func() bool {
		step++
		return s.stopImportAfter != nil && s.stopImportAfter(step)
	}

	if err := os.MkdirAll(filepath.Join(s.dir, stagingDir), 0o755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Join(s.dir, stagingDir), "import-")
	if err != nil {
		return err
	}
	committed := false
	stopped := false
	defer func() {
		if !committed && !stopped {
			os.RemoveAll(stage) // nothing was renamed: the Import never happened
		}
	}()

	var m manifestFile
	for i, rec := range records {
		rel, data, err := encode(rec, project)
		if err != nil {
			return err
		}
		name := strconv.Itoa(i)
		if err := writeSynced(filepath.Join(stage, name), data); err != nil {
			return err
		}
		m.Files = append(m.Files, manifestEntry{Staged: name, Path: filepath.ToSlash(rel)})
		if stopped = stop(); stopped {
			return errImportStopped
		}
	}
	data, err := encodeYAML(m)
	if err != nil {
		return err
	}
	if err := atomicWrite(filepath.Join(stage, manifestName), data); err != nil { // fsyncs the staging folder too
		return err
	}
	// From here the Import has happened: recovery rolls it forward.
	committed = true
	if stop() {
		return errImportStopped
	}
	return s.rollForward(stage, m, stop)
}

// rollForward renames each staged file still in staging into place, then
// deletes the staging folder. stop, when non-nil, is the test hook.
func (s *FileStore) rollForward(stage string, m manifestFile, stop func() bool) error {
	dirs := map[string]bool{}
	for _, e := range m.Files {
		dst := filepath.Join(s.dir, filepath.FromSlash(e.Path))
		dirs[filepath.Dir(dst)] = true
		src := filepath.Join(stage, e.Staged)
		if _, err := os.Stat(src); errors.Is(err, fs.ErrNotExist) {
			continue // renamed before a crash
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.Rename(src, dst); err != nil {
			return err
		}
		if stop != nil && stop() {
			return errImportStopped
		}
	}
	for d := range dirs {
		if err := syncDir(d); err != nil {
			return err
		}
	}
	if err := os.RemoveAll(stage); err != nil {
		return err
	}
	if err := syncDir(filepath.Dir(stage)); err != nil {
		return err
	}
	if stop != nil && stop() {
		return errImportStopped
	}
	return nil
}

// recoverImports runs at startup, before anything is loaded: each
// staging folder with a manifest is rolled forward, and one without is
// deleted.
func (s *FileStore) recoverImports() error {
	root := filepath.Join(s.dir, stagingDir)
	entries, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		stage := filepath.Join(root, e.Name())
		data, err := os.ReadFile(filepath.Join(stage, manifestName))
		if errors.Is(err, fs.ErrNotExist) {
			if err := os.RemoveAll(stage); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		var m manifestFile
		if field, err := decodeStrict(data, &m); err != nil {
			rel := filepath.ToSlash(filepath.Join(stagingDir, e.Name(), manifestName))
			return &LoadError{File: rel, Field: field, Msg: err.Error()}
		}
		for i, f := range m.Files {
			if f.Staged == "" || f.Path == "" || !filepath.IsLocal(f.Staged) || !filepath.IsLocal(filepath.FromSlash(f.Path)) {
				rel := filepath.ToSlash(filepath.Join(stagingDir, e.Name(), manifestName))
				return &LoadError{File: rel, Field: fmt.Sprintf("files[%d]", i), Msg: "needs a local staged name and path"}
			}
		}
		if err := s.rollForward(stage, m, nil); err != nil {
			return err
		}
	}
	return nil
}

// writeSynced writes a new file and fsyncs it.
func writeSynced(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
