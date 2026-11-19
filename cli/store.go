package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
	"gopkg.in/yaml.v3"
)

func init() {
	commands["store"] = command{
		usage: "store move --to <kind>[:<path>] [--url-env <name>] [--store <dir>]  copy the Store serve opens into a new, empty " +
			"files, sqlite or postgres Store and switch settings.yaml to it; offline: stop serve first",
		run: runStore,
	}
}

// copyStore is Store Move's copy; a test makes it go wrong.
var copyStore = store.Copy

func runStore(ctx context.Context, e Env, args []string) error {
	if len(args) == 0 {
		return usagef("expected move")
	}
	if args[0] != "move" {
		return usagef("unknown store command %q: expected move", args[0])
	}
	return storeMove(e, args[1:])
}

// storeMove is Store Move (ADR 0070): offline, it copies the Store serve
// would open into an empty one, checks the copy's counts, and then
// points settings.yaml's store: block at it. The source is left as it
// was, as a backup. It doesn't talk to the service: holding the source's
// lock is what keeps a running serve from writing during the copy.
func storeMove(e Env, args []string) error {
	fs := flagSet("store move")
	to := fs.String("to", "", "")
	urlEnv := fs.String("url-env", "", "")
	storeFlag := fs.String("store", "", "")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) > 0 {
		return usagef("unexpected argument %q", pos[0])
	}
	block, err := moveTarget(*to, *urlEnv, e.Dir)
	if err != nil {
		return err
	}
	h := e.home()
	local, err := loadSettings(h)
	if err != nil {
		return err
	}
	settingsPath, err := h.Config("code-central", "settings.yaml")
	if err != nil {
		return fmt.Errorf("cannot find settings.yaml to point at the new Store: %w", err)
	}
	from, err := storeLocation(*storeFlag, local, h)
	if err != nil {
		return err
	}
	target, err := blockLocation(block, h)
	if err != nil {
		return err
	}
	if from.Kind == target.Kind && from.Kind != store.Postgres && filepath.Clean(from.Path) == filepath.Clean(target.Path) {
		return fmt.Errorf("%s is the Store serve opens: move it to another place or Kind", target)
	}

	tally, err := move(e, from, target)
	if err != nil {
		return err
	}
	if err := writeStoreSetting(settingsPath, block); err != nil {
		return fmt.Errorf("the copy is complete, but settings.yaml couldn't be rewritten: %w; set its store: block to %s by hand", err, target)
	}

	if tally.Empty() {
		fmt.Fprintf(e.Stdout, "Moved %s into %s: it held nothing.\n", from, target)
	} else {
		fmt.Fprintf(e.Stdout, "Moved %s into %s:\n", from, target)
	}
	for _, key := range slices.Sorted(maps.Keys(tally.Projects)) {
		p := tally.Projects[key]
		fmt.Fprintf(e.Stdout, "  %s: %s, %s, %s, %s\n", key,
			counted(p.Specs, "Spec"), counted(p.Tickets, "Ticket"), counted(p.CIRuns, "CI Run"), counted(p.UsageDays, "Usage day"))
	}
	if tally.Options != domain.DefaultOptions() {
		fmt.Fprintln(e.Stdout, "  and the Options")
	}
	fmt.Fprintf(e.Stdout, "%s now names %s, which the next serve opens.\n", settingsPath, target)
	fmt.Fprintf(e.Stdout, "The source, %s, is untouched: keep it as a backup, or delete it once the new Store serves you.\n", from)
	switch {
	case *storeFlag != "":
		fmt.Fprintln(e.Stdout, "serve --store still opens the source: start serve without --store, and install the service again if it has one.")
	case e.Getenv("CODE_CENTRAL_STORE") != "":
		fmt.Fprintln(e.Stdout, "CODE_CENTRAL_STORE still names the source, and serve opens it first: unset it.")
	}
	return nil
}

// move copies the Store at from into the empty one at target and checks
// the copy, returning the Tally the two then share. It closes both.
func move(e Env, from, target store.Location) (store.Tally, error) {
	src, err := store.OpenAt(from)
	if errors.Is(err, store.ErrLocked) {
		return store.Tally{}, fmt.Errorf("%s is locked by a running code-central serve: stop the service and move again", from)
	}
	if err != nil {
		return store.Tally{}, fmt.Errorf("cannot open the source, %s: %w", from, err)
	}
	defer src.Close()
	if m := src.Migrated(); m != nil {
		fmt.Fprintf(e.Stderr, "code-central store move: migrated the source from format %d to %d, as serve would; the old store is kept at %s\n",
			m.From, m.To, m.Backup)
	}
	if err := store.CheckStopped(src, time.Now()); err != nil {
		return store.Tally{}, fmt.Errorf("%s: %w", from, err)
	}
	dst, err := store.OpenAt(target)
	if err != nil {
		return store.Tally{}, fmt.Errorf("cannot open the target, %s: %w", target, err)
	}
	defer dst.Close()
	before, err := store.TallyOf(dst)
	if err != nil {
		return store.Tally{}, fmt.Errorf("cannot read the target, %s: %w", target, err)
	}
	if !before.Empty() {
		return store.Tally{}, fmt.Errorf("%s isn't empty: Store Move copies only into an empty Store, never merging two", target)
	}
	partial := fmt.Sprintf("settings.yaml is unchanged and the source untouched; %s holds part of the copy: remove it before moving again", target)
	if err := copyStore(dst, src); err != nil {
		return store.Tally{}, fmt.Errorf("the copy stopped: %w; %s", err, partial)
	}
	// A Postgres source takes no lock: a service that started during the
	// copy would go on writing to it once the settings name the target.
	if err := store.CheckStopped(src, time.Now()); err != nil {
		return store.Tally{}, fmt.Errorf("%s: %w; %s", from, err, partial)
	}
	want, err := store.TallyOf(src)
	if err != nil {
		return store.Tally{}, fmt.Errorf("cannot count the source: %w; %s", err, partial)
	}
	got, err := store.TallyOf(dst)
	if err != nil {
		return store.Tally{}, fmt.Errorf("cannot count the copy: %w; %s", err, partial)
	}
	if diff := store.CompareTallies(want, got); len(diff) > 0 {
		return store.Tally{}, fmt.Errorf("the copy doesn't match the source: %s; %s", strings.Join(diff, "; "), partial)
	}
	if err := dst.Close(); err != nil {
		return store.Tally{}, fmt.Errorf("cannot close the copy: %w; %s", err, partial)
	}
	return want, nil
}

// moveTarget reads --to and --url-env as the store: block that names the
// target. A relative path is the working folder's.
func moveTarget(to, urlEnv, wd string) (storeSettings, error) {
	if to == "" {
		return storeSettings{}, usagef("--to is required: files, sqlite or postgres, with :<path> for files or sqlite")
	}
	kind, path, _ := strings.Cut(to, ":")
	b := storeSettings{Kind: kind}
	switch store.Kind(kind) {
	case store.Files, store.SQLite:
		if urlEnv != "" {
			return b, usagef("--url-env is for a postgres target; a %s target takes a path, as --to %s:<path>", kind, kind)
		}
		if path != "" && !filepath.IsAbs(path) {
			path = filepath.Join(wd, path)
		}
		b.Path = path
	case store.Postgres:
		if path != "" {
			return b, usagef("--to postgres takes no path: its URL is in the environment variable --url-env names (%s by default)", defaultURLEnv)
		}
		b.URLEnv = urlEnv
	default:
		return b, usagef("--to %s: the kind must be files, sqlite or postgres, not %q", to, kind)
	}
	return b, nil
}

// writeStoreSetting sets the store: block of the settings file at path to
// b, keeping its other settings and their comments, or writes a file
// holding only the block when there is none. It replaces the file whole.
func writeStoreSetting(path string, b storeSettings) error {
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return err
	}
	if doc.Kind == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return fmt.Errorf("%s doesn't hold settings", path)
	}
	var value yaml.Node
	if err := value.Encode(b); err != nil {
		return err
	}
	replaced := false
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "store" {
			root.Content[i+1] = &value
			replaced = true
		}
	}
	if !replaced {
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "store"}, &value)
	}
	var out bytes.Buffer
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	return replaceFile(path, out.Bytes())
}

// replaceFile writes data to path through a temporary file beside it,
// renamed over it, so a failure leaves the old file whole.
func replaceFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".settings-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(0o644); err != nil {
		f.Close()
		return err
	}
	if _, err := io.Copy(f, bytes.NewReader(data)); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

// counted is n things, as "1 Spec" or "2 Specs".
func counted(n int, thing string) string {
	if n == 1 {
		return "1 " + thing
	}
	return fmt.Sprintf("%d %ss", n, thing)
}
