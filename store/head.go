package store

import (
	"github.com/tomoram/code-central/domain"
)

// headFile is a Ticket's derived head fields (HeadOf) as a database
// Store keeps them beside the Ticket's document, so a read of a head
// loads no History. The File Store works them out on load instead.
type headFile struct {
	Entered              string             `yaml:"entered,omitempty"`
	LiveClaim            *claimFile         `yaml:"live_claim,omitempty"`
	LatestRound          *roundFile         `yaml:"latest_round,omitempty"`
	LatestFailureReport  *failureReportFile `yaml:"latest_failure_report,omitempty"`
	BackToReady          string             `yaml:"back_to_ready,omitempty"`
	ExternalStateChanged string             `yaml:"external_state_changed,omitempty"`
}

// headFileOf is h as its document, each part in its file shape.
func headFileOf(h domain.TicketHead) headFile {
	f := headFile{Entered: formatOptionalTime(h.Entered), BackToReady: formatOptionalTime(h.BackToReady), ExternalStateChanged: formatOptionalTime(h.ExternalStateChanged)}
	if h.LiveClaim != nil {
		f.LiveClaim = &encodeClaims([]domain.Claim{*h.LiveClaim})[0]
	}
	if h.LatestRound != nil {
		f.LatestRound = &encodeRounds([]domain.Round{*h.LatestRound})[0]
	}
	if h.LatestFailureReport != nil {
		f.LatestFailureReport = &encodeFailureReports([]domain.FailureReport{*h.LatestFailureReport})[0]
	}
	return f
}

// headOf is the derived head its document f holds.
func headOf(f headFile) (domain.TicketHead, *LoadError) {
	var h domain.TicketHead
	var err error
	if f.Entered != "" {
		if h.Entered, err = parseTime(f.Entered); err != nil {
			return h, &LoadError{Field: "entered", Msg: err.Error()}
		}
	}
	if f.BackToReady != "" {
		if h.BackToReady, err = parseTime(f.BackToReady); err != nil {
			return h, &LoadError{Field: "back_to_ready", Msg: err.Error()}
		}
	}
	if f.ExternalStateChanged != "" {
		if h.ExternalStateChanged, err = parseTime(f.ExternalStateChanged); err != nil {
			return h, &LoadError{Field: "external_state_changed", Msg: err.Error()}
		}
	}
	if f.LiveClaim != nil {
		cs, lerr := decodeClaims([]claimFile{*f.LiveClaim})
		if lerr != nil {
			return h, &LoadError{Field: "live_claim." + lerr.Field, Msg: lerr.Msg}
		}
		h.LiveClaim = &cs[0]
	}
	if f.LatestRound != nil {
		rs, lerr := decodeRounds([]roundFile{*f.LatestRound})
		if lerr != nil {
			return h, &LoadError{Field: "latest_round." + lerr.Field, Msg: lerr.Msg}
		}
		h.LatestRound = &rs[0]
	}
	if f.LatestFailureReport != nil {
		frs, lerr := decodeFailureReports([]failureReportFile{*f.LatestFailureReport})
		if lerr != nil {
			return h, &LoadError{Field: "latest_failure_report." + lerr.Field, Msg: lerr.Msg}
		}
		h.LatestFailureReport = &frs[0]
	}
	return h, nil
}
