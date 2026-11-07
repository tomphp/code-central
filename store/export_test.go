package store

// StopImportAfter makes s's next Imports stop dead after step k, as if
// the process had died there: nothing is cleaned up and memory is not
// updated. Steps are: stage each file, write the manifest, rename each
// file into place, delete the manifest.
func StopImportAfter(s *FileStore, k int) {
	s.stopImportAfter = func(step int) bool { return step == k }
}

// ErrImportStopped is what an Import stopped by StopImportAfter returns.
var ErrImportStopped = errImportStopped

// OpenFormat opens dir as a build whose format is format and whose
// migration chain is steps, so tests can migrate to a test-only format.
func OpenFormat(dir string, format int, steps []MigrationStep) (*FileStore, error) {
	return open(dir, format, steps)
}

// StopMigrationBetweenRenames makes the next migrations stop dead after
// the old store is renamed to its backup and before the staging folder
// is renamed into place, as a crash would. A nil t turns it off.
func StopMigrationBetweenRenames(t interface{ Cleanup(func()) }) {
	if t == nil {
		stopMigrationBetweenRenames = false
		return
	}
	stopMigrationBetweenRenames = true
	t.Cleanup(func() { stopMigrationBetweenRenames = false })
}

// ErrMigrationStopped is what a migration stopped by the hook returns.
var ErrMigrationStopped = errMigrationStopped
