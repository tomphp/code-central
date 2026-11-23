package core

import (
	"context"
	"slices"
	"strings"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/core/foundation"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// Focus Kinds (Spec code-central-550): the human chooses per Project which
// Needs Me reasons are Focus Items with set_focus_kinds. The Project's
// evaluation marks each Needs Me item a Focus Item or not, and filters
// nothing: Needs Me, its count, the needs-you reason on Tickets and
// Notifications still include the reasons that are off.

// focusItem reports whether an item waiting for reason is a Focus Item
// in Project p: whether p's Focus Kinds leave reason on.
func focusItem(p domain.Project, reason api.NeedsMeReason) bool {
	return !slices.Contains(p.FocusKindsOff, string(reason))
}

func focusKindsView(off []string) api.FocusKindsView {
	v := api.FocusKindsView{Off: []api.NeedsMeReason{}}
	for _, r := range off {
		v.Off = append(v.Off, api.NeedsMeReason(r))
	}
	return v
}

// SetFocusKinds is the human's set_focus_kinds: it replaces the Project's
// list of reasons that are off whole, collapsing duplicates. Like the
// Project's other settings it adds no Move, Edit or Comment; the
// Project's notice announces it, and only on a change.
func (c *Core) SetFocusKinds(ctx context.Context, key string, in api.SetFocusKindsInput) (api.Written[api.ProjectView], error) {
	if err := foundation.HumanOnly(ctx, "set_focus_kinds"); err != nil {
		return api.Written[api.ProjectView]{}, err
	}
	var before, after api.ProjectView
	err := c.store.Update(func(w store.Writer) error {
		if err := foundation.Writable(w, key); err != nil {
			return err
		}
		off, err := checkedFocusKindsOff(in.Off)
		if err != nil {
			return err
		}
		p, _ := w.Project(key)
		before = c.projectView(p)
		was := p.FocusKindsOff
		p.FocusKindsOff = off
		after = c.projectView(p)
		if slices.Equal(off, was) {
			return nil
		}
		return w.PutProject(p)
	})
	if err != nil {
		return api.Written[api.ProjectView]{}, err
	}
	return acknowledgeProject(before, after, in.Full, c.now())
}

// checkedFocusKindsOff checks every reason is a Needs Me reason and
// returns them as stored: in group order, each once.
func checkedFocusKindsOff(in []api.NeedsMeReason) ([]string, error) {
	var off []string
	for _, r := range in {
		if groupIndex(r) < 0 {
			var names []string
			for _, x := range api.NeedsMeReasonViews {
				names = append(names, string(x.Reason))
			}
			return nil, api.InvalidField("off", "%q isn't a Needs Me reason; the reasons are %s", r, strings.Join(names, ", "))
		}
		if !slices.Contains(off, string(r)) {
			off = append(off, string(r))
		}
	}
	slices.SortFunc(off, func(a, b string) int {
		return groupIndex(api.NeedsMeReason(a)) - groupIndex(api.NeedsMeReason(b))
	})
	return off, nil
}
