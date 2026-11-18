package ratelimit

import "testing"

func TestAErrorNamesTheCISystem(t *testing.T) {
	err := &Error{System: "GitHub", Message: "API rate limit exceeded"}
	if got, want := err.Error(), "GitHub is rate-limiting gh: API rate limit exceeded"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

func TestAErrorOfAnotherSystemNamesItWithoutGh(t *testing.T) {
	err := &Error{System: "Azure DevOps", Message: "too many"}
	if got, want := err.Error(), "Azure DevOps is throttling Code Central: too many"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}
