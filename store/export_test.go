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
