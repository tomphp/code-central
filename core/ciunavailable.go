package core

import "time"

// CI unavailable (code-central-262): a Ticket the CI gate holds shows in
// Needs Me once it has waited CIUnavailableAfter since its push while
// nothing is telling Code Central how its CI went: the CI Watcher isn't
// Watching, or no CI Runs have been reported for its pushed commit.
// Worked out on read by the Project's CI state, never stored.

// CIUnavailableAfter is how long after its push a Ticket the CI gate
// holds may go without word of its CI before it is CI unavailable.
const CIUnavailableAfter = 10 * time.Minute
