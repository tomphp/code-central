// Command cifailures trims the output of `gh run view <id> --log-failed`
// to what explains the failure. It reads the log on standard input and
// drops each line's job, step and timestamp prefix, colour codes, the
// setup groups (the step's command, shell and env), blank lines, and the
// lines that report a test starting or passing. What is left is the
// failing tests' output, capped at -max lines. It is a developer tool for
// the Makefile's ci-failures target, so it lives under internal/.
//
//	gh run view 123 --log-failed 2>&1 | go run ./internal/cifailures
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

var (
	// prefix is what gh puts before every line: the job, the step and the
	// runner's timestamp, separated by tabs and a space.
	prefix = regexp.MustCompile(`^[^\t]*\t[^\t]*\t\x{feff}?\d{4}-\d\d-\d\dT[\d:.]+Z ?`)
	colour = regexp.MustCompile(`\x1b\[[0-9;]*m`)
	// noise is a line that says a test started, paused, resumed or passed.
	noise = regexp.MustCompile(`^\s*(=== (RUN|PAUSE|CONT|NAME)\b|--- PASS|PASS$|ok  \t|\?   \t|✓)`)
)

func main() {
	max := flag.Int("max", 120, "the most lines to print")
	flag.Parse()
	trim(os.Stdin, os.Stdout, *max)
}

// trim writes the lines of the log in that explain a failure to out,
// stopping after max lines.
func trim(in io.Reader, out io.Writer, max int) {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	printed, dropped, group := 0, 0, false
	for sc.Scan() {
		line := colour.ReplaceAllString(prefix.ReplaceAllString(sc.Text(), ""), "")
		switch {
		case strings.HasPrefix(line, "##[group]"):
			group = true
			continue
		case strings.HasPrefix(line, "##[endgroup]"):
			group = false
			continue
		case group, strings.TrimSpace(line) == "", noise.MatchString(line):
			continue
		}
		if printed == max {
			dropped++
			continue
		}
		fmt.Fprintln(out, line)
		printed++
	}
	if dropped > 0 {
		fmt.Fprintf(out, "... %d more lines; run `gh run view <id> --log-failed` for all of them\n", dropped)
	}
}
