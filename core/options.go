package core

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// Options (Spec code-central-160): the human's global preferences, kept
// in the store apart from any Project. Any Actor reads them; only the
// human changes them, and only over HTTP. A change adds no Move, Edit or
// Comment: the Options file is its only record, and options.changed its
// only notice.

// GetOptions is get_options: the Options as they stand.
func (c *Core) GetOptions(ctx context.Context) (api.OptionsView, error) {
	var v api.OptionsView
	err := c.store.View(func(r store.Reader) error {
		v = optionsView(r.Options())
		return nil
	})
	return v, err
}

// SetOptions is the human's set_options. It validates every change before
// writing, and writes only when something changes.
func (c *Core) SetOptions(ctx context.Context, in api.SetOptionsInput) (api.OptionsView, error) {
	if err := humanOnly(ctx, "set_options"); err != nil {
		return api.OptionsView{}, err
	}
	if in.FocusClear != nil && !in.FocusClear.Valid() {
		return api.OptionsView{}, api.InvalidField("focus_clear", "focus_clear %q is not an Option's value; allowed: %s", *in.FocusClear, strings.Join(domain.FocusClearNames(), ", "))
	}
	if in.FocusPopup != nil && !in.FocusPopup.Valid() {
		return api.OptionsView{}, api.InvalidField("focus_popup", "focus_popup %q is not an Option's value; allowed: %s", *in.FocusPopup, strings.Join(domain.FocusPopupNames(), ", "))
	}
	if in.LimitsPopup != nil && !in.LimitsPopup.Valid() {
		return api.OptionsView{}, api.InvalidField("limits_popup", "limits_popup %q is not an Option's value; allowed: %s", *in.LimitsPopup, strings.Join(domain.LimitsPopupNames(), ", "))
	}
	if in.GHPath != nil {
		gh := strings.TrimSpace(*in.GHPath)
		if gh != "" && !filepath.IsAbs(gh) {
			return api.OptionsView{}, api.InvalidField("gh_path", "gh path %q isn't an absolute path, such as /opt/homebrew/bin/gh", gh)
		}
		in.GHPath = &gh
	}
	var v api.OptionsView
	err := c.store.Update(func(w store.Writer) error {
		old := w.Options()
		o := old
		if in.FocusClear != nil {
			o.FocusClear = *in.FocusClear
		}
		if in.FocusPopup != nil {
			o.FocusPopup = *in.FocusPopup
		}
		if in.LimitsPopup != nil {
			o.LimitsPopup = *in.LimitsPopup
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

func optionsView(o domain.Options) api.OptionsView {
	return api.OptionsView{FocusClear: o.FocusClear, FocusPopup: o.FocusPopup, LimitsPopup: o.LimitsPopup, GHPath: o.GHPath}
}
