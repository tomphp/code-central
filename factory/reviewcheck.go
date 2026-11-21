package factory

import (
	"context"
	"encoding/json"
	"path"
	"strings"

	"github.com/tomoram/code-central/api"
)

// calledReview reports whether a tool call is the Skill tool running the
// code-review skill, under any plugin prefix (mattpocock-skills:code-review).
func calledReview(tool string, input json.RawMessage) bool {
	if tool != "Skill" {
		return false
	}
	var in struct {
		Skill string `json:"skill"`
	}
	if json.Unmarshal(input, &in) != nil {
		return false
	}
	name := in.Skill
	if i := strings.LastIndex(name, ":"); i >= 0 {
		name = name[i+1:]
	}
	return name == "code-review"
}

// docsOnly reports whether every changed file is under docs/ or is a
// Markdown file.
func docsOnly(files []string) bool {
	for _, f := range files {
		if !strings.HasPrefix(f, "docs/") && path.Ext(f) != ".md" {
			return false
		}
	}
	return true
}

// checkReview comments on tk, which t's Claim ended with a finish, when
// it is an implementation finish with a branch, no turn of the Claim
// called the code-review skill (reviewed) and the branch's diff since its
// base touches more than docs. The human sees it at In Review.
func (f *factory) checkReview(ctx context.Context, t *turn, tk api.TicketView, reviewed bool) {
	c := t.claim
	if reviewed || tk.Branch == "" || api.GuideName(tk.Labels) != api.ImplementationStage {
		return
	}
	r := f.repos.home
	base := f.repos.baseOf(r)
	if tk.SpecLanded && tk.Spec.Branch != "" {
		base = tk.Spec.Branch
	}
	files, err := f.repos.git.Open(r.dir).ChangedFiles("refs/heads/"+base, "refs/heads/"+tk.Branch)
	if err != nil {
		f.claimf(c, "%s: can't diff %s against %s to check for a review: %v", c.Ticket, tk.Branch, base, err)
		return
	}
	if len(files) == 0 || docsOnly(files) {
		return
	}
	body := "code-central-factory: this finish skipped the code-review skill. No turn of the Claim (session " + c.SessionID +
		") called it, and the branch changes files beyond docs and Markdown. Run a review of " + tk.Branch + " before accepting it, or send the Ticket back."
	if _, err := f.client.Comment(ctx, f.project, api.CommentInput{Ticket: api.Ref(c.Ticket), Body: body}); err != nil {
		f.claimf(c, "%s: comment: %v", c.Ticket, err)
	}
}
