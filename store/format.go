//go:build !migrationtest

package store

// Format is the store format this build reads and writes. Bump it (by
// raising currentFormat) for every schema change, even an additive one,
// and add the step from the old number to migrations (migrate.go). See
// core's schema snapshot test.
const Format = currentFormat
