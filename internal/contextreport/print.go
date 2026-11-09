// Printing a report, with each figure's change against a baseline.

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
// different numbers of sessions compare fairly; a kind with no sessions
// in the report has nothing to compare, and says so.
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

	p.line("\nTool results per session, by kind of session")
	p.row("KIND", "SESSIONS", "TOTAL", "PER SESSION", p.column())
	for _, name := range kindNames(r, base) {
		now, was := kind(r, name), p.baseKind(name)
		if now.Sessions == 0 {
			p.row(name, "0", "", "", fmt.Sprintf("no sessions (was %s per session)", size(per(was.Bytes, was.Sessions))))
			continue
		}
		p.row(name, fmt.Sprint(now.Sessions), size(float64(now.Bytes)), size(per(now.Bytes, now.Sessions)), p.against(per(now.Bytes, now.Sessions), per(was.Bytes, was.Sessions), size))
	}
	p.figures("Tools, per session, by kind of session (Code Central's operations are listed next)", r, func(k Kind) []Figure { return k.Tools })
	p.figures("Code Central operations, per session, by kind of session", r, func(k Kind) []Figure { return k.Operations })

	p.line("\nLargest tool results")
	for _, res := range r.Largest {
		tool := res.Tool
		if op, ok := operation(tool); ok {
			tool = op
		}
		p.row(size(float64(res.Bytes)), res.Kind, res.Ticket, tool, res.Input, p.largestChange(res))
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
	p.row("Reads again within a session", count(per(r.WithinSession, r.Sessions)), p.against(per(r.WithinSession, r.Sessions), per(was.WithinSession, was.Sessions), count))
	p.row("Reads again across sessions", count(per(r.AcrossSessions, r.Sessions)), p.against(per(r.AcrossSessions, r.Sessions), per(was.AcrossSessions, was.Sessions), count))

	p.line("\nSigns a cut went too far, per session, by kind of session")
	p.row("KIND", "WIDER READS", p.column(), "ROUNDS", p.column(), "QUESTIONS", p.column())
	for _, name := range kindNames(r, base) {
		now, was := kind(r, name), p.baseKind(name)
		if now.Sessions == 0 {
			p.row(name, "no sessions", "", "", "", "", "")
			continue
		}
		cells := []string{name}
		for _, f := range []func(Kind) int{
			func(k Kind) int { return k.WiderReads },
			func(k Kind) int { return k.Rounds },
			func(k Kind) int { return k.Questions },
		} {
			cells = append(cells, count(per(f(now), now.Sessions)), p.against(per(f(now), now.Sessions), per(f(was), was.Sessions), count))
		}
		p.row(cells...)
	}
	if len(r.WiderReads) > 0 {
		p.line("\nWider reads: reads called again for more")
	}
	for _, w := range r.WiderReads {
		p.row(w.Ticket, w.Kind, w.Operation+" "+w.ID, w.From, "then "+w.To)
	}
	return p.tw.Flush()
}

// figures prints the tools or operations of each of the report's kinds,
// listing those only the baseline has after the report's own.
func (p printer) figures(title string, r Report, list func(Kind) []Figure) {
	p.line("\n%s", title)
	for _, now := range r.Kinds {
		was := p.baseKind(now.Name)
		nowFigures, wasFigures := list(now), list(was)
		if len(nowFigures) == 0 && len(wasFigures) == 0 {
			continue
		}
		p.row(now.Name, "CALLS", p.column(), "SIZE", p.column())
		for _, name := range union(nowFigures, wasFigures) {
			n, w := find(nowFigures, name), find(wasFigures, name)
			p.row("  "+name,
				count(per(n.Calls, now.Sessions)), p.against(per(n.Calls, now.Sessions), per(w.Calls, was.Sessions), count),
				size(per(n.Bytes, now.Sessions)), p.against(per(n.Bytes, now.Sessions), per(w.Bytes, was.Sessions), size))
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

func (p printer) baseKind(name string) Kind {
	if p.base == nil {
		return Kind{}
	}
	return kind(*p.base, name)
}

// against is now's change from was, or empty without a baseline.
func (p printer) against(now, was float64, format func(float64) string) string {
	if p.base == nil {
		return ""
	}
	return change(now, was, format)
}

// change is now's change from was: unchanged, a percentage with was, or
// new when was is zero.
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

// kindNames is the report's kinds, then any only the baseline has,
// which are listed as having no sessions.
func kindNames(r Report, base *Report) []string {
	var names []string
	for _, k := range r.Kinds {
		names = append(names, k.Name)
	}
	if base != nil {
		for _, k := range base.Kinds {
			if kind(r, k.Name).Name == "" {
				names = append(names, k.Name)
			}
		}
	}
	return names
}

func kind(r Report, name string) Kind {
	for _, k := range r.Kinds {
		if k.Name == name {
			return k
		}
	}
	return Kind{}
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

// per is n per session, or zero when there are none.
func per(n, sessions int) float64 {
	if sessions == 0 {
		return 0
	}
	return float64(n) / float64(sessions)
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
