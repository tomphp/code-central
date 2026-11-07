package store

import "testing"

// The real chain has exactly one step from each older format, in order,
// so any older store reaches Format.
func TestTheMigrationChainReachesFormat(t *testing.T) {
	if len(migrations) != Format-1 {
		t.Fatalf("%d migration steps for format %d: add one step from each older format to migrations (migrate.go)", len(migrations), Format)
	}
	for i, st := range migrations {
		if st.From != i+1 || st.Name == "" {
			t.Fatalf("migrations[%d] = {From: %d, Name: %q}, want From %d and a name", i, st.From, st.Name, i+1)
		}
	}
}
