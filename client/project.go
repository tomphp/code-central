package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MCPFile is the file init writes and shell commands read the key from.
const MCPFile = ".mcp.json"

// ServerName is the .mcp.json server entry init owns.
const ServerName = "code-central"

// ServiceURL is the service every client talks to: CODE_CENTRAL_URL, else
// DefaultURL.
func ServiceURL(getenv func(string) string) string {
	if u := getenv("CODE_CENTRAL_URL"); u != "" {
		return u
	}
	return DefaultURL
}

// ResolveProject is the Project Key every client of the service resolves:
// projectFlag, else CODE_CENTRAL_PROJECT, else the key in the nearest
// .mcp.json at or above dir. It is "" when none of them names one.
func ResolveProject(dir, projectFlag string, getenv func(string) string) (string, error) {
	if projectFlag != "" {
		return projectFlag, nil
	}
	if p := getenv("CODE_CENTRAL_PROJECT"); p != "" {
		return p, nil
	}
	return nearestKey(dir)
}

// ReadMCP reads .mcp.json as a generic document, so a merge keeps every
// other key. A missing file is an empty document.
func ReadMCP(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	doc := map[string]any{}
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("%s isn't valid JSON (%v); fix it and run init again", path, err)
	}
	return doc, nil
}

// KeyIn is the Project Key in a .mcp.json document's code-central entry.
func KeyIn(doc map[string]any) (string, bool) {
	servers, _ := doc["mcpServers"].(map[string]any)
	entry, _ := servers[ServerName].(map[string]any)
	u, _ := entry["url"].(string)
	i := strings.LastIndex(u, "/p/")
	if i < 0 || !strings.HasSuffix(u, "/mcp") {
		return "", false
	}
	key := strings.TrimSuffix(u[i+len("/p/"):], "/mcp")
	if key == "" || strings.Contains(key, "/") {
		return "", false
	}
	return key, true
}

// nearestKey is the Project Key in the nearest .mcp.json at or above dir
// that has a code-central entry, or "" if there is none.
func nearestKey(dir string) (string, error) {
	for d := dir; d != ""; {
		path := filepath.Join(d, MCPFile)
		if _, err := os.Stat(path); err == nil {
			doc, err := ReadMCP(path)
			if err != nil {
				return "", err
			}
			if k, ok := KeyIn(doc); ok {
				return k, nil
			}
		}
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
		d = parent
	}
	return "", nil
}
