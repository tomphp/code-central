package cli

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/tomoram/code-central/core"
)

// Failure Reports and giving up (ticket 30).

// runReportFailure takes the four fields as flags, or as one JSON object
// {what_broke, why, what_to_fix, where_work_stands} through --file (a
// path, or - for stdin). A flag given beside --file overrides its field.
func runReportFailure(ctx context.Context, e Env, args []string) error {
	fs := flagSet("report-failure")
	claim := fs.String("claim", "", "")
	whatBroke := fs.String("what-broke", "", "")
	why := fs.String("why", "", "")
	whatToFix := fs.String("what-to-fix", "", "")
	whereWorkStands := fs.String("where-work-stands", "", "")
	file := fs.String("file", "", "")
	sc, ref, _, err := itemCommand(e, fs, args, 0, 0)
	if err != nil {
		return err
	}
	in := core.ReportFailureInput{}
	if *file != "" {
		text, err := longInput(e, "", *file)
		if err != nil {
			return err
		}
		dec := json.NewDecoder(bytes.NewReader([]byte(text)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&in); err != nil {
			return usagef("--file must hold a JSON object with what_broke, why, what_to_fix and where_work_stands: %v", err)
		}
	}
	for name, p := range map[string]struct{ dst, src *string }{
		"what-broke":        {&in.WhatBroke, whatBroke},
		"why":               {&in.Why, why},
		"what-to-fix":       {&in.WhatToFix, whatToFix},
		"where-work-stands": {&in.WhereWorkStands, whereWorkStands},
	} {
		if isSet(fs, name) {
			*p.dst = *p.src
		}
	}
	in.Ticket, in.ClaimID = ref, *claim
	v, err := sc.client.ReportFailure(ctx, sc.project, in)
	if err != nil {
		return err
	}
	return printJSON(e, v)
}

func runGiveUp(ctx context.Context, e Env, args []string) error {
	fs := flagSet("give-up")
	claim := fs.String("claim", "", "")
	note := fs.String("note", "", "")
	file := fs.String("file", "", "")
	sc, ref, _, err := itemCommand(e, fs, args, 0, 0)
	if err != nil {
		return err
	}
	text, err := longInput(e, *note, *file)
	if err != nil {
		return err
	}
	v, err := sc.client.GiveUp(ctx, sc.project, core.GiveUpInput{Ticket: ref, ClaimID: *claim, Note: text})
	if err != nil {
		return err
	}
	return printJSON(e, v)
}

// runSendBack sends an Accepted Ticket back to Ready, as the agent
// (ticket 72).
func runSendBack(ctx context.Context, e Env, args []string) error {
	fs := flagSet("send-back")
	note := fs.String("note", "", "")
	file := fs.String("file", "", "")
	full := fs.Bool("full", false, "")
	sc, ref, _, err := itemCommand(e, fs, args, 0, 0)
	if err != nil {
		return err
	}
	text, err := longInput(e, *note, *file)
	if err != nil {
		return err
	}
	v, err := sc.client.SendBack(ctx, sc.project, core.SendBackInput{Ticket: ref, Note: text, Full: *full})
	if err != nil {
		return err
	}
	return printJSON(e, v)
}
