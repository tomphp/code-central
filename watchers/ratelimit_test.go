package watchers

import "testing"

func TestARateLimitErrorNamesTheCISystem(t *testing.T) {
	err := &RateLimitError{System: "GitHub", Message: "API rate limit exceeded"}
	if got, want := err.Error(), "GitHub is rate-limiting gh: API rate limit exceeded"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}
