// Package gh is how every Watcher reaches GitHub: through gh, the GitHub
// CLI, and its own login, so Code Central stores no GitHub token. It finds
// gh, checks its login, calls GitHub's REST API through gh api, and
// recognises GitHub refusing a call for its rate limit.
package gh

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/textproto"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tomoram/code-central/core"
)

// Env is everything a Watcher touches outside itself to reach gh, so
// tests can run it in-process.
type Env struct {
	// Environ is the environment: it is read for PATH and ProgramFiles,
	// and given to gh.
	Environ []string
	// InstallPlaces are the folders gh is looked for in when it isn't on
	// PATH; nil means DefaultInstallPlaces.
	InstallPlaces []string
}

func (e Env) getenv(key string) string {
	for i := len(e.Environ) - 1; i >= 0; i-- {
		if k, v, ok := strings.Cut(e.Environ[i], "="); ok && k == key {
			return v
		}
	}
	return ""
}

// DefaultInstallPlaces are where gh's installers put it: Homebrew on
// Apple silicon and Intel Macs, Homebrew on Linux, and the Windows
// installer under %ProgramFiles%.
func DefaultInstallPlaces(getenv func(string) string) []string {
	places := []string{"/opt/homebrew/bin", "/usr/local/bin", "/home/linuxbrew/.linuxbrew/bin"}
	if pf := getenv("ProgramFiles"); pf != "" {
		places = append(places, filepath.Join(pf, "GitHub CLI"))
	}
	return places
}

// missing is the problem of a gh found nowhere it was looked for.
func missing(reason string) core.WatcherCheckResult {
	return core.WatcherCheckResult{Problem: core.ProblemGHMissing, Reason: reason,
		Fix: "Install gh, the GitHub CLI (https://cli.github.com), or set the gh path Option to where it is."}
}

// Find finds gh: ghPath, the gh path Option, if set, else on PATH, else
// in the install places. Not finding it is the gh missing problem.
func Find(ghPath string, env Env) (string, *core.WatcherCheckResult) {
	if ghPath != "" {
		if prog, ok := isProgram(ghPath); ok {
			return prog, nil
		}
		if prog, ok := in(ghPath); ok {
			return prog, nil
		}
		r := missing(fmt.Sprintf("The gh path Option is %s, but gh isn't there.", ghPath))
		r.Fix = "Set the gh path Option to where gh is, or clear it to find gh on the service's PATH."
		return "", &r
	}
	for _, dir := range filepath.SplitList(env.getenv("PATH")) {
		if prog, ok := in(dir); ok {
			return prog, nil
		}
	}
	places := env.InstallPlaces
	if places == nil {
		places = DefaultInstallPlaces(env.getenv)
	}
	for _, dir := range places {
		if prog, ok := in(dir); ok {
			return prog, nil
		}
	}
	r := missing(fmt.Sprintf("gh, the GitHub CLI, isn't on the service's PATH or in %s.", strings.Join(places, ", ")))
	return "", &r
}

// in is gh in the folder dir, if it is there.
func in(dir string) (string, bool) {
	if dir == "" {
		return "", false
	}
	return isProgram(filepath.Join(dir, "gh"+exeSuffix))
}

// isProgram reports whether path is a program that can be run.
func isProgram(path string) (string, bool) {
	fi, err := os.Stat(path)
	return path, err == nil && !fi.IsDir() && executable(fi)
}

// CheckLogin checks prog, gh, is logged in to GitHub with gh auth status:
// a result with no Problem when it is, else the problem and its fix.
// GitHub rate-limiting the check is a *RateLimitError, not a problem.
func CheckLogin(ctx context.Context, env Env, prog string) (core.WatcherCheckResult, error) {
	cmd, stdout, stderr := command(ctx, env, prog, "auth", "status", "--hostname", "github.com")
	err := cmd.Run()
	if err == nil {
		return core.WatcherCheckResult{}, nil
	}
	if r, ran := DidntRun(prog, err); !ran {
		return r, nil
	}
	// gh writes its errors to stderr.
	out := stdout.String()
	if strings.TrimSpace(stderr.String()) != "" {
		out = stderr.String()
	}
	// gh auth status calls GitHub too, and says so when that is
	// rate-limited, with no reset time.
	if strings.Contains(strings.ToLower(out), "rate limit") {
		return core.WatcherCheckResult{}, &RateLimitError{Message: rateLimitLine(out)}
	}
	return core.WatcherCheckResult{Problem: core.ProblemLoggedOut, Reason: "gh isn't logged in to GitHub: " + FirstLine(out),
		Fix: "Run `gh auth login` as the user the service runs as."}, nil
}

// command is prog, gh, run with args in env, never prompting, writing
// to stdout and stderr.
func command(ctx context.Context, env Env, prog string, args ...string) (cmd *exec.Cmd, stdout, stderr *bytes.Buffer) {
	cmd = exec.CommandContext(ctx, prog, args...)
	cmd.Env = append(append([]string{}, env.Environ...), "GH_PROMPT_DISABLED=1", "NO_COLOR=1")
	stdout, stderr = &bytes.Buffer{}, &bytes.Buffer{}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	return cmd, stdout, stderr
}

// rateLimitLine is the line of out that names the rate limit.
func rateLimitLine(out string) string {
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(strings.ToLower(l), "rate limit") {
			return strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(l), "-X "))
		}
	}
	return FirstLine(out)
}

// DidntRun is a check failing because prog, gh, didn't run at all,
// rather than running and saying no; ran is true when it did run.
func DidntRun(prog string, err error) (r core.WatcherCheckResult, ran bool) {
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.Exited() {
		return r, true
	}
	return core.WatcherCheckResult{Problem: core.ProblemCheckFailed, Reason: fmt.Sprintf("gh at %s didn't run: %v", prog, err),
		Fix: "Check gh runs for the user the service runs as, or set the gh path Option to one that does."}, false
}

// FirstLine is s's first line with text on it, trimmed.
func FirstLine(s string) string {
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			return l
		}
	}
	return ""
}

// ErrNotFound is GitHub answering a call with 404 Not Found.
var ErrNotFound = errors.New("not found")

// API runs prog, gh, as gh api path with args, never prompting, and with
// --include, so the response's headers say when a rate limit ends. It
// returns the response's body and, when it fails, the first line gh wrote
// to stderr; GitHub refusing it for a rate limit is a *RateLimitError,
// and answering 404 is ErrNotFound.
func API(ctx context.Context, env Env, prog, path string, args ...string) (body []byte, msg string, err error) {
	cmd, stdout, stderr := command(ctx, env, prog, append(append([]string{"api", path}, args...), "--include")...)
	err = cmd.Run()
	status, header, body := response(stdout.Bytes())
	if err == nil {
		return body, "", nil
	}
	msg = FirstLine(stderr.String())
	if limit := rateLimit(status, header, msg); limit != nil {
		return nil, msg, limit
	}
	if status == http.StatusNotFound {
		return nil, msg, fmt.Errorf("%w: %w", ErrNotFound, err)
	}
	return nil, msg, err
}

// response splits what gh api --include prints into the status, the
// headers and the body. Output with no status line is all body.
func response(out []byte) (status int, header http.Header, body []byte) {
	if !bytes.HasPrefix(out, []byte("HTTP/")) {
		return 0, http.Header{}, out
	}
	r := bufio.NewReader(bytes.NewReader(out))
	tp := textproto.NewReader(r)
	line, err := tp.ReadLine()
	if err != nil {
		return 0, http.Header{}, out
	}
	if _, code, ok := strings.Cut(line, " "); ok {
		code, _, _ = strings.Cut(code, " ")
		status, _ = strconv.Atoi(code)
	}
	h, err := tp.ReadMIMEHeader()
	if err != nil && !errors.Is(err, io.EOF) {
		return status, http.Header{}, out
	}
	body, _ = io.ReadAll(r)
	return status, http.Header(h), body
}

// RateLimitError is GitHub refusing a call for its rate limit.
type RateLimitError struct {
	// Reset is when GitHub said the limit ends, and RetryAfter how long
	// it said to wait; both are zero when it said neither.
	Reset      time.Time
	RetryAfter time.Duration
	// Message is what gh said.
	Message string
}

func (e *RateLimitError) Error() string { return "GitHub is rate-limiting gh: " + e.Message }

// Until is when a Watcher may call GitHub again, from now; zero when
// GitHub didn't say.
func (e *RateLimitError) Until(now time.Time) time.Time {
	if e.RetryAfter > 0 {
		return now.Add(e.RetryAfter)
	}
	return e.Reset
}

// CheckResult is a check failing because GitHub rate-limited it, at now:
// there is no knowing yet whether the Watcher can watch, so it is the
// check failed problem, with when to try again.
func (e *RateLimitError) CheckResult(now time.Time) core.WatcherCheckResult {
	r := core.WatcherCheckResult{Problem: core.ProblemCheckFailed, Reason: e.Error(), Fix: "Try again in a few minutes."}
	if until := e.Until(now); !until.IsZero() {
		r.Fix = "Try again after " + until.Local().Format(time.Kitchen) + ", when GitHub's rate limit ends."
	}
	return r
}

// rateLimit is the rate limit a failed call's answer says GitHub refused
// it for, or nil: a 403 or 429 with no requests remaining, or one saying
// how long to wait, or a message naming a rate limit, as GitHub's
// secondary rate limits may give with neither.
func rateLimit(status int, header http.Header, msg string) *RateLimitError {
	if status != 0 && status != http.StatusForbidden && status != http.StatusTooManyRequests {
		return nil
	}
	limit := &RateLimitError{Message: msg}
	limited := strings.Contains(strings.ToLower(msg), "rate limit")
	if secs, err := strconv.Atoi(header.Get("Retry-After")); err == nil && secs > 0 {
		limit.RetryAfter, limited = time.Duration(secs)*time.Second, true
	}
	if header.Get("X-Ratelimit-Remaining") == "0" {
		limited = true
		if reset, err := strconv.ParseInt(header.Get("X-Ratelimit-Reset"), 10, 64); err == nil && reset > 0 {
			limit.Reset = time.Unix(reset, 0).UTC()
		}
	}
	if !limited {
		return nil
	}
	return limit
}
