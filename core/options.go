package core

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/tomoram/code-central/store"
)

// Options (Spec code-central-160): the human's global preferences, kept
// in the store apart from any Project. Any Actor reads them; only the
// human changes them, and only over HTTP. A change adds no Move, Edit or
// Comment: the Options file is its only record, and options.changed its
// only notice.

// OptionsView is the Options as every front door shows them.
type OptionsView struct {
	FocusClear store.FocusClear `json:"focus_clear"`
	// FocusPopup is the Focus Pop-up Option.
	FocusPopup store.FocusPopup `json:"focus_popup"`
	// GHPath is the gh path Option: where gh is, "" to find it on the
	// service's PATH, then in the usual install places.
	GHPath string `json:"gh_path"`
}

// SetOptionsInput is the Options to change; each left out stays as it is.
type SetOptionsInput struct {
	FocusClear *store.FocusClear `json:"focus_clear,omitempty"`
	FocusPopup *store.FocusPopup `json:"focus_popup,omitempty"`
	// GHPath is an absolute path, or "" to clear it.
	GHPath *string `json:"gh_path,omitempty"`
}

// GetOptions is get_options: the Options as they stand.
func (c *Core) GetOptions(ctx context.Context) (OptionsView, error) {
	var v OptionsView
	err := c.store.View(func(r store.Reader) error {
		v = optionsView(r.Options())
		return nil
	})
	return v, err
}

// SetOptions is the human's set_options. It validates every change before
// writing, and writes only when something changes.
func (c *Core) SetOptions(ctx context.Context, in SetOptionsInput) (OptionsView, error) {
	if err := humanOnly(ctx, "set_options"); err != nil {
		return OptionsView{}, err
	}
	if in.FocusClear != nil && !in.FocusClear.Valid() {
		return OptionsView{}, invalid("focus_clear", "focus_clear %q is not an Option's value; allowed: %s", *in.FocusClear, strings.Join(store.FocusClearNames(), ", "))
	}
	if in.FocusPopup != nil && !in.FocusPopup.Valid() {
		return OptionsView{}, invalid("focus_popup", "focus_popup %q is not an Option's value; allowed: %s", *in.FocusPopup, strings.Join(store.FocusPopupNames(), ", "))
	}
	if in.GHPath != nil {
		gh := strings.TrimSpace(*in.GHPath)
		if gh != "" && !filepath.IsAbs(gh) {
			return OptionsView{}, invalid("gh_path", "gh path %q isn't an absolute path, such as /opt/homebrew/bin/gh", gh)
		}
		in.GHPath = &gh
	}
	var v OptionsView
	err := c.store.Update(func(w store.Writer) error {
		old := w.Options()
		o := old
		if in.FocusClear != nil {
			o.FocusClear = *in.FocusClear
		}
		if in.FocusPopup != nil {
			o.FocusPopup = *in.FocusPopup
		}
		if in.GHPath != nil {
			o.GHPath = *in.GHPath
		}
		v = optionsView(o)
		if o == old {
			return nil
		}
		return w.PutOptions(o)
	})
	return v, err
}

// OptionNames is every Option there is, for the refusal of an unknown one.
var OptionNames = []string{"focus_clear", "focus_popup", "gh_path"}

func optionsView(o store.Options) OptionsView {
	return OptionsView{FocusClear: o.FocusClear, FocusPopup: o.FocusPopup, GHPath: o.GHPath}
}
