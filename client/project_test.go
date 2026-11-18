package client

import (
	"os"
	"path/filepath"
	"testing"
)

func envOf(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func writeMCPFile(t *testing.T, dir, url string) {
	t.Helper()
	body := `{"mcpServers":{"code-central":{"type":"http","url":"` + url + `"}}}`
	if err := os.WriteFile(filepath.Join(dir, MCPFile), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestServiceURL(t *testing.T) {
	if got := ServiceURL(envOf(nil)); got != DefaultURL {
		t.Errorf("unset: got %q, want %q", got, DefaultURL)
	}
	if got := ServiceURL(envOf(map[string]string{"CODE_CENTRAL_URL": "http://x:1"})); got != "http://x:1" {
		t.Errorf("set: got %q", got)
	}
}

func TestResolveProjectPrefersFlagThenEnvironmentThenMCPFile(t *testing.T) {
	root := t.TempDir()
	writeMCPFile(t, root, "${CODE_CENTRAL_URL:-http://localhost:7733}/p/fromfile/mcp")
	sub := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	env := envOf(map[string]string{"CODE_CENTRAL_PROJECT": "fromenv"})
	for _, tc := range []struct {
		name, flag string
		getenv     func(string) string
		want       string
	}{
		{"flag", "fromflag", env, "fromflag"},
		{"environment", "", env, "fromenv"},
		{"nearest .mcp.json above the directory", "", envOf(nil), "fromfile"},
	} {
		got, err := ResolveProject(sub, tc.flag, tc.getenv)
		if err != nil || got != tc.want {
			t.Errorf("%s: got %q, %v; want %q", tc.name, got, err, tc.want)
		}
	}
}

func TestResolveProjectNearestMCPFileWins(t *testing.T) {
	root := t.TempDir()
	writeMCPFile(t, root, "http://h/p/outer/mcp")
	inner := filepath.Join(root, "inner")
	if err := os.Mkdir(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	writeMCPFile(t, inner, "http://h/p/inner/mcp")
	if got, _ := ResolveProject(inner, "", envOf(nil)); got != "inner" {
		t.Errorf("got %q, want inner", got)
	}
}

func TestResolveProjectSkipsFilesWithoutAKeyAndReportsBadJSON(t *testing.T) {
	root := t.TempDir()
	writeMCPFile(t, root, "http://h/p/outer/mcp")
	inner := filepath.Join(root, "inner")
	if err := os.Mkdir(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inner, MCPFile), []byte(`{"mcpServers":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, _ := ResolveProject(inner, "", envOf(nil)); got != "outer" {
		t.Errorf("got %q, want outer", got)
	}
	if err := os.WriteFile(filepath.Join(inner, MCPFile), []byte(`{nope`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveProject(inner, "", envOf(nil)); err == nil {
		t.Error("bad JSON: want an error")
	}
}

func TestKeyIn(t *testing.T) {
	doc := func(url string) map[string]any {
		return map[string]any{"mcpServers": map[string]any{ServerName: map[string]any{"url": url}}}
	}
	for url, want := range map[string]string{
		"http://h/p/acme/mcp": "acme",
		"http://h/p/a/b/mcp":  "",
		"http://h/p//mcp":     "",
		"http://h/other":      "",
	} {
		if got, ok := KeyIn(doc(url)); got != want || ok != (want != "") {
			t.Errorf("%s: got %q, %v", url, got, ok)
		}
	}
	if _, ok := KeyIn(map[string]any{}); ok {
		t.Error("empty document has no key")
	}
}
