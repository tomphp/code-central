package domain

// BranchTag is the reserved Spec Tag whose value names the Spec's Spec
// Branch in place of spec/<spec id> (Spec code-central-1089). On a
// Ticket it means nothing and isn't checked.
const BranchTag = "branch"

// SpecBranchPrefix starts every default Spec Branch name, so no branch
// Tag may start with it.
const SpecBranchPrefix = "spec/"

// SpecBranch is the Spec Branch of the Spec id, a display ID, carrying
// tags: its branch Tag's value when it has one, else spec/<id>. It is
// the one place the name is made.
func SpecBranch(id string, tags map[string]string) string {
	if name := tags[BranchTag]; name != "" {
		return name
	}
	return SpecBranchPrefix + id
}
