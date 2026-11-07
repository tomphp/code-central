package storetest

import (
	"slices"
	"testing"

	"github.com/tomoram/code-central/store"
)

// runReplacements checks that a Cancelled Blocker is followed through its
// Replacements when read, so cancelling never rewrites another Ticket.
func runReplacements(t *testing.T, f Factory) {
	t.Run("replacements are followed on read without rewriting dependants", func(t *testing.T) {
		s := f.New(t)
		seedProject(t, s)
		tk := func(n int, state store.State, blockedBy, replacements []int) store.Ticket {
			return store.Ticket{Number: n, Title: "T", State: state, Spec: 1, BlockedBy: blockedBy, Replacements: replacements, Created: t0, Updated: t0}
		}
		put(t, s, func(w store.Writer) error {
			return w.PutSpec("acme", store.Spec{Number: 1, Title: "S", Created: t0, Updated: t0})
		})
		for _, x := range []store.Ticket{
			tk(2, store.Ready, []int{3}, nil),     // blocked by 3
			tk(3, store.Ready, nil, nil),          // cancelled below
			tk(4, store.Ready, nil, nil),          // a Replacement
			tk(5, store.Cancelled, nil, []int{6}), // a Cancelled Replacement, itself replaced
			tk(6, store.Draft, nil, nil),
			tk(7, store.Cancelled, nil, nil),      // Cancelled with no Replacements
			tk(8, store.Cancelled, nil, []int{9}), // a loop (only by hand edits)
			tk(9, store.Cancelled, nil, []int{8}),
			tk(10, store.Cancelled, nil, []int{11, 12}), // a diamond: both sides reach 13
			tk(11, store.Cancelled, nil, []int{13}),
			tk(12, store.Cancelled, nil, []int{13}),
			tk(13, store.Ready, nil, nil),
		} {
			put(t, s, func(w store.Writer) error { return w.PutTicket("acme", x) })
		}
		// Cancelling 3 writes 3's file only.
		put(t, s, func(w store.Writer) error { return w.PutTicket("acme", tk(3, store.Cancelled, nil, []int{4, 5})) })

		check := func(s store.Store) {
			read(t, s, func(r store.Reader) {
				for _, c := range []struct {
					n    int
					want []int
				}{
					{3, []int{4, 6}}, // followed through 5 to 6
					{4, []int{4}},    // not Cancelled: itself
					{7, []int{7}},    // no Replacements: itself, still blocking
					{8, []int{9}},    // a loop stops at the Ticket that closes it, which still blocks
					{10, []int{13}},
					{42, []int{42}}, // unknown: itself
				} {
					if got := store.Follow(r, "acme", c.n); !slices.Equal(got, c.want) {
						t.Errorf("Follow(%d) = %v, want %v", c.n, got, c.want)
					}
				}
				if d, _ := r.Ticket("acme", 2); !slices.Equal(d.BlockedBy, []int{3}) {
					t.Errorf("dependant's blocked_by = %v, want [3] unchanged", d.BlockedBy)
				}
				if c, _ := r.Ticket("acme", 3); !slices.Equal(c.Replacements, []int{4, 5}) {
					t.Errorf("replacements = %v, want [4 5]", c.Replacements)
				}
			})
		}
		check(s)
		if f.Reopen != nil {
			check(f.Reopen(t, s))
		}
	})
}
