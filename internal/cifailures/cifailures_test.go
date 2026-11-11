package main

import (
	"strings"
	"testing"
)

const log = "test (ubuntu-latest)\tUI e2e\t\ufeff2026-11-11T12:46:02.4082044Z ##[group]Run npm run test:e2e\n" +
	"test (ubuntu-latest)\tUI e2e\t2026-11-11T12:46:02.4111709Z env:\n" +
	"test (ubuntu-latest)\tUI e2e\t2026-11-11T12:46:02.4112212Z ##[endgroup]\n" +
	"test (ubuntu-latest)\tUI e2e\t2026-11-11T12:46:02.4906669Z \n" +
	"test (ubuntu-latest)\tUI e2e\t2026-11-11T12:46:02.4906670Z === RUN   TestA\n" +
	"test (ubuntu-latest)\tUI e2e\t2026-11-11T12:46:02.4906671Z --- PASS: TestA (0.00s)\n" +
	"test (ubuntu-latest)\tUI e2e\t2026-11-11T12:46:02.4906672Z \x1b[31m--- FAIL: TestB\x1b[0m\n" +
	"test (ubuntu-latest)\tUI e2e\t2026-11-11T12:46:02.4906673Z     b_test.go:9: got 1, want 2\n"

func trimmed(max int) string {
	var out strings.Builder
	trim(strings.NewReader(log), &out, max)
	return out.String()
}

func TestOnlyTheFailingOutputRemains(t *testing.T) {
	want := "--- FAIL: TestB\n    b_test.go:9: got 1, want 2\n"
	if got := trimmed(100); got != want {
		t.Errorf("trimmed log is\n%q\nwant\n%q", got, want)
	}
}

func TestTheOutputIsCappedAndSaysSo(t *testing.T) {
	got := trimmed(1)
	if !strings.HasPrefix(got, "--- FAIL: TestB\n... 1 more lines") {
		t.Errorf("capped log is %q", got)
	}
}
