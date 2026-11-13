package core

import (
	"context"
	"slices"
	"strings"

	"github.com/tomoram/code-central/store"
)

// Focus Kinds (Spec code-central-550): the human chooses per Project which
// Needs Me reasons are Focus Items with set_focus_kinds. Only the web's
// focus queue reads them; Needs Me, its count, the needs-you reason on
// Tickets and Notifications don't.

// FocusKindsView is a Project's Focus Kinds as every Project read returns
// them: the Needs Me reasons that are off, in group order, empty while
// every reason is on.
type FocusKindsView struct {
	Off []NeedsMeReason `json:"off"`
}

func focusKindsView(off []string) FocusKindsView {
	v := FocusKindsView{Off: []NeedsMeReason{}}
	for _, r := range off {
		v.Off = append(v.Off, NeedsMeReason(r))
	}
	return v
}

// SetFocusKindsInput is the human's set_focus_kinds(off, full?).
type SetFocusKindsInput struct {
	// Off is the whole list of Needs Me reasons to turn off; every other
	// reason is on.
	Off []NeedsMeReason `json:"off"`
	// Full asks for the Project in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// SetFocusKinds is the human's set_focus_kinds: it replaces the Project's
// list of reasons that are off whole, collapsing duplicates. Like the
// Project's other settings it adds no Move, Edit or Comment; the
// Project's notice announces it, and only on a change.
func (c *Core) SetFocusKinds(ctx context.Context, key string, in SetFocusKindsInput) (Written[ProjectView], error) {
	if err := humanOnly(ctx, "set_focus_kinds"); err != nil {
		return Written[ProjectView]{}, err
	}
	off, err := checkedFocusKindsOff(in.Off)
	if err != nil {
		return Written[ProjectView]{}, err
	}
	var before, after ProjectView
	err = c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
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
		return Written[ProjectView]{}, err
	}
	return acknowledgeProject(before, after, in.Full, c.now())
}

// checkedFocusKindsOff checks every reason is a Needs Me reason and
// returns them as stored: in group order, each once.
func checkedFocusKindsOff(in []NeedsMeReason) ([]string, error) {
	var off []string
	for _, r := range in {
		if groupIndex(r) < 0 {
			var names []string
			for _, x := range needsMeReasons {
				names = append(names, string(x.Reason))
			}
			return nil, invalid("off", "%q isn't a Needs Me reason; the reasons are %s", r, strings.Join(names, ", "))
		}
		if !slices.Contains(off, string(r)) {
			off = append(off, string(r))
		}
	}
	slices.SortFunc(off, func(a, b string) int {
		return groupIndex(NeedsMeReason(a)) - groupIndex(NeedsMeReason(b))
	})
	return off, nil
}
