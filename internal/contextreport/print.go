package main

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"
)

// printer writes a report, and with a baseline each figure's change
// against it. Figures are compared per session, so windows with
// different numbers of sessions compare fairly; a Stage with no
// sessions in the report has nothing to compare beyond that.
type printer struct {
	tw   *tabwriter.Writer
	base *Report
}

// write prints the report r to out, against base when it isn't nil.
func write(out io.Writer, r Report, base *Report) error {
	p := printer{tw: tabwriter.NewWriter(out, 0, 0, 2, ' ', 0), base: base}
	p.line("Context report: %s", span(r))
	if base != nil {
		p.line("Baseline: %s", span(*base))
	}

	p.line("\nTool results per session, by Stage")
	p.row("STAGE", "SESSIONS", "TOTAL", "PER SESSION", p.column())
	for _, name := range stageNames(r, base) {
		now, was := stage(r, name), p.baseStage(name)
		if now.Sessions == 0 {
			p.row(name, "0", "", "", fmt.Sprintf("no sessions (was %s per session)", size(perSession(was.Bytes, was))))
			continue
		}
		p.row(name, fmt.Sprint(now.Sessions), size(float64(now.Bytes)), size(perSession(now.Bytes, now)), p.change(perSession(now.Bytes, now), perSession(was.Bytes, was), size))
	}
	p.figures("Tools, per session, by Stage (Code Central's operations are listed next)", r, func(s Stage) []Figure { return s.Tools })
	p.figures("Code Central operations, per session, by Stage", r, func(s Stage) []Figure { return s.Operations })

	p.line("\nLargest tool results")
	for _, res := range r.Largest {
		p.row(size(float64(res.Bytes)), res.Stage, res.Ticket, strings.TrimPrefix(res.Tool, codeCentral), res.Input, p.largestChange(res))
	}

	p.line("\nSpecs and Tickets read more than once")
	for _, rr := range r.ReReads {
		p.row(rr.ID, fmt.Sprintf("%d reads in %d sessions", rr.Reads, rr.Sessions), strings.Join(rr.Readers, ", "))
	}
	p.line("")
	var was Report
	if base != nil {
		was = *base
	}
	p.row("Reads again within a session", count(perReport(r.WithinSession, r)), p.change(perReport(r.WithinSession, r), perReport(was.WithinSession, was), count))
	p.row("Reads again across sessions", count(perReport(r.AcrossSessions, r)), p.change(perReport(r.AcrossSessions, r), perReport(was.AcrossSessions, was), count))

	p.line("\nSigns a cut went too far, per session, by Stage")
	p.row("STAGE", "READS CALLED AGAIN FOR MORE", p.column(), "ROUNDS", p.column(), "QUESTIONS", p.column())
	for _, now := range r.Stages {
		was := p.baseStage(now.Name)
		cells := []string{now.Name}
		for _, f := range []func(Stage) int{
			func(s Stage) int { return s.Widenings },
			func(s Stage) int { return s.Rounds },
			func(s Stage) int { return s.Questions },
		} {
			cells = append(cells, count(perSession(f(now), now)), p.change(perSession(f(now), now), perSession(f(was), was), count))
		}
		p.row(cells...)
	}
	if len(r.Widenings) > 0 {
		p.line("\nReads called again for more")
	}
	for _, w := range r.Widenings {
		p.row(w.Ticket, w.Stage, w.Operation+" "+w.ID, w.From, "then "+w.To)
	}
	return p.tw.Flush()
}

// figures prints each of the report's Stages' tools or operations,
// listing those only the baseline has after the report's own.
func (p printer) figures(title string, r Report, list func(Stage) []Figure) {
	p.line("\n%s", title)
	for _, now := range r.Stages {
		was := p.baseStage(now.Name)
		nowFigures, wasFigures := list(now), list(was)
		if len(nowFigures) == 0 && len(wasFigures) == 0 {
			continue
		}
		p.line("%s", now.Name)
		for _, f := range union(nowFigures, wasFigures) {
			n, w := find(nowFigures, f), find(wasFigures, f)
			p.row("  "+f, count(perSession(n.Calls, now))+" calls", size(perSession(n.Bytes, now)), p.change(perSession(n.Bytes, now), perSession(w.Bytes, was), size))
		}
	}
}

func (p printer) line(format string, args ...any) {
	fmt.Fprintf(p.tw, format+"\n", args...)
}

func (p printer) row(cells ...string) {
	fmt.Fprintln(p.tw, strings.Join(cells, "\t"))
}

// column is a change column's heading, empty without a baseline.
func (p printer) column() string {
	if p.base == nil {
		return ""
	}
	return "CHANGE"
}

func (p printer) baseStage(name string) Stage {
	if p.base == nil {
		return Stage{}
	}
	return stage(*p.base, name)
}

// change is now's change from was: unchanged, a percentage with was, or
// new when the baseline had none. It is empty without a baseline.
func (p printer) change(now, was float64, format func(float64) string) string {
	if p.base == nil {
		return ""
	}
	return change(now, was, format)
}

func change(now, was float64, format func(float64) string) string {
	switch {
	case format(now) == format(was):
		return "unchanged"
	case was == 0:
		return "new"
	}
	return fmt.Sprintf("%+.0f%% (was %s)", (now-was)/was*100, format(was))
}

// largestChange compares a result with the baseline's largest result of
// the same tool and input, if the baseline lists one.
func (p printer) largestChange(res Result) string {
	if p.base == nil {
		return ""
	}
	for _, was := range p.base.Largest {
		if was.Tool == res.Tool && was.Input == res.Input {
			return change(float64(res.Bytes), float64(was.Bytes), size)
		}
	}
	return "new"
}

func span(r Report) string {
	return fmt.Sprintf("%d sessions started from %s to %s", r.Sessions, r.From.UTC().Format(time.RFC3339), r.To.UTC().Format(time.RFC3339))
}

// stageNames is the report's Stages, then any only the baseline has,
// which the Stage table lists as having no sessions.
func stageNames(r Report, base *Report) []string {
	var names []string
	for _, s := range r.Stages {
		names = append(names, s.Name)
	}
	if base != nil {
		for _, s := range base.Stages {
			if stage(r, s.Name).Name == "" {
				names = append(names, s.Name)
			}
		}
	}
	return names
}

func stage(r Report, name string) Stage {
	for _, s := range r.Stages {
		if s.Name == name {
			return s
		}
	}
	return Stage{}
}

func union(now, was []Figure) []string {
	var names []string
	for _, f := range now {
		names = append(names, f.Name)
	}
	for _, f := range was {
		if find(now, f.Name).Name == "" {
			names = append(names, f.Name)
		}
	}
	return names
}

func find(figures []Figure, name string) Figure {
	for _, f := range figures {
		if f.Name == name {
			return f
		}
	}
	return Figure{}
}

func perSession(n int, s Stage) float64 {
	if s.Sessions == 0 {
		return 0
	}
	return float64(n) / float64(s.Sessions)
}

func perReport(n int, r Report) float64 {
	if r.Sessions == 0 {
		return 0
	}
	return float64(n) / float64(r.Sessions)
}

// size formats bytes as B, KB or MB.
func size(bytes float64) string {
	switch {
	case bytes < 1024:
		return fmt.Sprintf("%.0fB", bytes)
	case bytes < 1024*1024:
		return fmt.Sprintf("%.1fKB", bytes/1024)
	}
	return fmt.Sprintf("%.1fMB", bytes/1024/1024)
}

// count formats a count per session to three significant digits, so a
// small one isn't shown as zero.
func count(n float64) string {
	if n >= 1000 {
		return fmt.Sprintf("%.0f", n)
	}
	return fmt.Sprintf("%.3g", n)
}
