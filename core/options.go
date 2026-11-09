package core

import (
	"context"
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
}

// SetOptionsInput is the Options to change; each left out stays as it is.
type SetOptionsInput struct {
	FocusClear *store.FocusClear `json:"focus_clear,omitempty"`
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
		var allowed []string
		for _, f := range store.FocusClears {
			allowed = append(allowed, string(f))
		}
		return OptionsView{}, invalid("focus_clear", "focus_clear %q is not an Option's value; allowed: %s", *in.FocusClear, strings.Join(allowed, ", "))
	}
	var v OptionsView
	err := c.store.Update(func(w store.Writer) error {
		old := w.Options()
		o := old
		if in.FocusClear != nil {
			o.FocusClear = *in.FocusClear
		}
		v = optionsView(o)
		if o == old {
			return nil
		}
		return w.PutOptions(o)
	})
	return v, err
}

func optionsView(o store.Options) OptionsView {
	return OptionsView{FocusClear: o.FocusClear}
}
