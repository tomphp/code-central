package server_test

// Golden snapshots of what the three front doors serve: the MCP tools/list,
// the HTTP route table and the CLI usage. A change to any of them fails with
// a diff; when the change is intended, regenerate the goldens with
//   go test ./server -run Snapshot -write

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/tomoram/code-central/cli"
	"github.com/tomoram/code-central/httpapi"
)

var writeSnapshots = flag.Bool("write", false, "rewrite the golden snapshots in server/testdata")

// matchesGolden compares got with testdata/<name>, rewriting it under -write.
func matchesGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *writeSnapshots {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s", path)
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v\nrun: go test ./server -run Snapshot -write", err)
	}
	if string(want) != got {
		t.Fatalf("%s changed (-golden +served):\n%s\nIf intended, run: go test ./server -run Snapshot -write",
			path, lineDiff(string(want), got))
	}
}

// lineDiff lists the lines only one side has, in order, with - and +.
func lineDiff(want, got string) string {
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	// Longest common subsequence, so a moved or edited line shows as one - and one +.
	lcs := make([][]int, len(w)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(g)+1)
	}
	for i := len(w) - 1; i >= 0; i-- {
		for j := len(g) - 1; j >= 0; j-- {
			if w[i] == g[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var out strings.Builder
	i, j := 0, 0
	for i < len(w) || j < len(g) {
		switch {
		case i < len(w) && j < len(g) && w[i] == g[j]:
			i++
			j++
		case j < len(g) && (i == len(w) || lcs[i][j+1] >= lcs[i+1][j]):
			fmt.Fprintf(&out, "+ %s\n", g[j])
			j++
		default:
			fmt.Fprintf(&out, "- %s\n", w[i])
			i++
		}
	}
	return out.String()
}

func TestMCPToolsListSnapshot(t *testing.T) {
	srv := newServer(t)
	s := connect(t, srv, "acme")
	res, err := s.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	tools := res.Tools
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	type tool struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		InputSchema any    `json:"inputSchema"`
	}
	var served []tool
	for _, tl := range tools {
		served = append(served, tool{tl.Name, tl.Description, tl.InputSchema})
	}
	// Marshalling sorts map keys, so the schema's order is stable.
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(served); err != nil {
		t.Fatal(err)
	}
	matchesGolden(t, "mcp-tools-list.json", buf.String())
}

func TestHTTPRouteTableSnapshot(t *testing.T) {
	routes := httpapi.Routes()
	names := make([]string, 0, len(routes))
	for name := range routes {
		names = append(names, name)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, name := range names {
		method, pattern, _ := strings.Cut(routes[name], " ")
		fmt.Fprintf(&b, "%s\t%s\t%s\n", name, method, pattern)
	}
	matchesGolden(t, "http-routes.txt", b.String())
}

func TestCLIUsageSnapshot(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cli.Run(context.Background(), []string{"help"}, cli.Env{
		Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errOut,
		Getenv: func(string) string { return "" },
	})
	if code != 0 || errOut.Len() != 0 {
		t.Fatalf("help exited %d with stderr %q", code, errOut.String())
	}
	matchesGolden(t, "cli-usage.txt", out.String())
}
