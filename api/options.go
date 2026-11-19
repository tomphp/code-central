package api

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/tomoram/code-central/domain"
)

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

// OptionNames is every Option there is, for the refusal of an unknown one.
func OptionNames() []string { return []string{"focus_clear", "focus_popup", "limits_popup", "gh_path"} }
