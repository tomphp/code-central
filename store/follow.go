package store

import "slices"

// Follow returns the Tickets standing in for Ticket n as a Blocker: n
// itself, unless it is Cancelled with Replacements, in which case each
// Replacement followed in turn. Being blocked by a Cancelled Ticket is
// resolved here, on read, so cancelling never rewrites another Ticket.
//
// A Cancelled Ticket with no Replacements stands for itself, and keeps
// its dependants blocked. A Replacement chain that loops back on itself
// (possible only by hand edits) stops at the Ticket that closes the loop.
func Follow(r Reader, project string, n int) []int {
	return follow(r, project, n, map[int]bool{})
}

func follow(r Reader, project string, n int, seen map[int]bool) []int {
	if seen[n] {
		return nil
	}
	seen[n] = true // on the current path only, so two routes to one Ticket both reach it
	defer delete(seen, n)
	t, ok := r.Ticket(project, n)
	if !ok || t.State != Cancelled || len(t.Replacements) == 0 {
		return []int{n}
	}
	var out []int
	for _, rep := range t.Replacements {
		for _, m := range follow(r, project, rep, seen) {
			if !slices.Contains(out, m) {
				out = append(out, m)
			}
		}
	}
	if len(out) == 0 {
		return []int{n}
	}
	return out
}
