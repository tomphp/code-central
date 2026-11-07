//go:build migrationtest

package store

// A migrationtest build plays the next release: its format is one past
// the real chain, reached by a test-only additive step, so the e2e tier
// can migrate a real store with the real binary. Never ship it.
const Format = currentFormat + 1

func init() {
	migrations = append(migrations, MigrationStep{From: currentFormat, Name: "test-only additive step"})
}
