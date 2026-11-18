package core

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// Options (Spec code-central-160): the human's global preferences, kept
// in the store apart from any Project. Any Actor reads them; only the
// human changes them, and only over HTTP. A change adds no Move, Edit or
// Comment: the Options file is its only record, and options.changed its
// only notice.

// OptionsView is the Options as every front door shows them.
type OptionsView struct {
	FocusClear domain.FocusClear `json:"focus_clear"`
	// FocusPopup is the Focus Pop-up Option.
	FocusPopup domain.FocusPopup `json:"focus_popup"`
	// LimitsPopup is the Show Limits popup Option.
	LimitsPopup domain.LimitsPopup `json:"limits_popup"`
	// GHPath is the gh path Option: where gh is, "" to find it on the
	// service's PATH, then in the usual install places.
	GHPath string `json:"gh_path"`
}

// SetOptionsInput is the Options to change; each left out stays as it is.
type SetOptionsInput struct {
	FocusClear  *domain.FocusClear  `json:"focus_clear,omitempty"`
	FocusPopup  *domain.FocusPopup  `json:"focus_popup,omitempty"`
	LimitsPopup *domain.LimitsPopup `json:"limits_popup,omitempty"`
	// GHPath is an absolute path, or "" to clear it.
	GHPath *string `json:"gh_path,omitempty"`
}

// DescribeDecodeError names the Options there are when the body has a key
// that is not one; the HTTP door calls it on a body that fails to decode.
func (*SetOptionsInput) DescribeDecodeError(err error) error {
	var e *Error
	if errors.As(err, &e) && e.Field != "" && !slices.Contains(OptionNames(), e.Field) {
		e.Message += fmt.Sprintf("; %q is not an Option (the Options are: %s)", e.Field, strings.Join(OptionNames(), ", "))
	}
	return err
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
		return OptionsView{}, invalid("focus_clear", "focus_clear %q is not an Option's value; allowed: %s", *in.FocusClear, strings.Join(domain.FocusClearNames(), ", "))
	}
	if in.FocusPopup != nil && !in.FocusPopup.Valid() {
		return OptionsView{}, invalid("focus_popup", "focus_popup %q is not an Option's value; allowed: %s", *in.FocusPopup, strings.Join(domain.FocusPopupNames(), ", "))
	}
	if in.LimitsPopup != nil && !in.LimitsPopup.Valid() {
		return OptionsView{}, invalid("limits_popup", "limits_popup %q is not an Option's value; allowed: %s", *in.LimitsPopup, strings.Join(domain.LimitsPopupNames(), ", "))
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

// OptionNames is every Option there is, for the refusal of an unknown one.
func OptionNames() []string { return []string{"focus_clear", "focus_popup", "limits_popup", "gh_path"} }

func optionsView(o domain.Options) OptionsView {
	return OptionsView{FocusClear: o.FocusClear, FocusPopup: o.FocusPopup, LimitsPopup: o.LimitsPopup, GHPath: o.GHPath}
}
