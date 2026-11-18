// The Store interface a Store Kind implements to keep usage (ADR 0069),
// which a Recorder works over.

package usage

import "fmt"

// Record is one usage record to append: exactly one of Call and Response
// is set.
type Record struct {
	Call     *Call
	Response *Response
}

// Skipped is a place a read passed over lines it couldn't parse.
type Skipped struct {
	// Where names the file or table the lines were in.
	Where string
	Lines int
}

// Store is where usage is kept: a Store Kind's form of ADR 0016's
// records. The File Store keeps JSON Lines files, one per Project per
// local day; the in-memory fake keeps them in memory. A Store is safe for
// concurrent use and takes no lock of the Store's Updates, so an append
// never waits on a write and a write never waits on an append.
//
// Days are local days as YYYY-MM-DD, which the caller (the Recorder) works
// out from its clock and zone.
type Store interface {
	// Append adds r to project's records for day.
	Append(project, day string, r Record) error
	// Read returns the records of project, or of every Project when
	// project is "", for each of days, which it takes in the order given:
	// within a day, Projects in key order, each Project's calls and
	// responses in the order appended. Responses carry their Project and
	// Day. It returns what it could read: a record it can't parse is
	// counted in Skipped and left out, and the error, if any, names what it
	// couldn't read at all. Records.Days is left for the caller.
	Read(project string, days []string) (Records, error)
	// Days are the days project has records for, oldest first: none for a
	// Project with no Usage.
	Days(project string) ([]string, error)
	// Prune deletes every Project's records for days earlier than before.
	// It deletes what it can and returns an error for what it couldn't.
	Prune(before string) error
}

// errAppend names what an Append couldn't do.
func errAppend(project, day string, err error) error {
	return fmt.Errorf("%s's usage for %s: %w", project, day, err)
}
