package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tomoram/code-central/client"
	"github.com/tomoram/code-central/core"
)

// mcpFile is the file init writes and shell commands read the key from.
const mcpFile = ".mcp.json"

// serverName is the .mcp.json server entry init owns.
const serverName = "code-central"

// mcpURL is the server URL for a Project Key, exactly as written into
// .mcp.json. The shape is stable across versions.
func mcpURL(key string) string {
	return "${CODE_CENTRAL_URL:-" + client.DefaultURL + "}/p/" + key + "/mcp"
}

// runInit sets the repository (or folder) containing e.Dir up as a
// Project. It talks to the service, writes .mcp.json and never commits.
func runInit(ctx context.Context, e Env, args []string) error {
	fs := flagSet("init")
	keyFlag := fs.String("key", "", "")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) > 0 {
		return usagef("unexpected argument %q", pos[0])
	}
	root, inGit := repoRoot(e.Dir)
	c := newClient(e)
	in := bufio.NewReader(e.Stdin)
	ask := func(prompt string) (string, error) {
		fmt.Fprint(e.Stdout, prompt)
		line, err := in.ReadString('\n')
		if err != nil && line == "" { // end of input: nobody to answer
			fmt.Fprintln(e.Stdout)
			return "", usagef("init cancelled; nothing was written")
		}
		return strings.TrimSpace(line), nil
	}

	path := filepath.Join(root, mcpFile)
	doc, err := readMCP(path)
	if err != nil {
		return err
	}
	proposed := ""
	if k, ok := keyIn(doc); ok {
		switch _, err := c.GetProject(ctx, k); {
		case err == nil:
			fmt.Fprintf(e.Stdout, "%s already points at Project %s; nothing to do.\n", path, k)
			return nil
		case !isCode(err, core.UnknownProject):
			return err
		}
		proposed = k
	}
	if proposed == "" {
		proposed = proposeKey(root, inGit)
	}

	key := *keyFlag
	if key == "" {
		answer, err := ask(fmt.Sprintf("Project Key [%s]: ", proposed))
		if err != nil {
			return err
		}
		key = answer
		if key == "" {
			key = proposed
		}
	}
	for {
		_, err := c.GetProject(ctx, key)
		if err == nil {
			answer, err := ask(fmt.Sprintf("Project %s already exists. Join it? Only if this repository is that Project. [y/N]: ", key))
			if err != nil {
				return err
			}
			if yes(answer, false) {
				break
			}
			if key, err = ask("Choose another Project Key: "); err != nil {
				return err
			}
			continue
		}
		if !isCode(err, core.UnknownProject) {
			return err
		}
		answer, err := ask(fmt.Sprintf("Create Project %s? [Y/n]: ", key))
		if err != nil {
			return err
		}
		if !yes(answer, true) {
			return usagef("init cancelled; nothing was written")
		}
		if _, err := c.CreateProject(ctx, core.CreateProjectInput{Key: key}); err != nil {
			var ce *core.Error
			if errors.As(err, &ce) && ce.Code == core.Invalid {
				fmt.Fprintln(e.Stdout, ce.Message)
				if key, err = ask("Choose another Project Key: "); err != nil {
					return err
				}
				continue
			}
			return err
		}
		fmt.Fprintf(e.Stdout, "Created Project %s.\n", key)
		break
	}

	if err := writeMCP(path, doc, key); err != nil {
		return err
	}
	hooks, err := writeHooks(root)
	if err != nil {
		return err
	}
	fmt.Fprintf(e.Stdout, "Wrote %s and %s; nothing was committed.\n", path, hooks)
	return nil
}

// yes reads a y/n answer, with def for an empty one.
func yes(answer string, def bool) bool {
	switch strings.ToLower(answer) {
	case "":
		return def
	case "y", "yes":
		return true
	}
	return false
}

func isCode(err error, code core.Code) bool {
	var ce *core.Error
	return errors.As(err, &ce) && ce.Code == code
}

func serviceURL(e Env) string {
	if u := e.Getenv("CODE_CENTRAL_URL"); u != "" {
		return u
	}
	return client.DefaultURL
}

// gitOut runs git in dir and returns its trimmed output.
func gitOut(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// repoRoot is the top of the git repository containing dir, or dir itself
// outside git (or without git installed).
func repoRoot(dir string) (string, bool) {
	top, err := gitOut(dir, "rev-parse", "--show-toplevel")
	if err != nil || top == "" {
		return dir, false
	}
	return top, true
}

// proposeKey proposes a Project Key: the repository name from the only
// remote, else from origin, else the folder name; lowercased.
func proposeKey(root string, inGit bool) string {
	if inGit {
		if out, err := gitOut(root, "remote"); err == nil && out != "" {
			names := strings.Fields(out)
			remote := ""
			if len(names) == 1 {
				remote = names[0]
			} else {
				for _, n := range names {
					if n == "origin" {
						remote = n
					}
				}
			}
			if remote != "" {
				if u, err := gitOut(root, "remote", "get-url", remote); err == nil {
					if name := repoName(u); name != "" {
						return strings.ToLower(name)
					}
				}
			}
		}
	}
	return strings.ToLower(filepath.Base(root))
}

// repoName is the repository name alone from a remote URL, such as "api"
// from git@github.com:acme/api.git or https://github.com/acme/api/.
func repoName(remote string) string {
	s := strings.TrimRight(remote, "/")
	s = strings.TrimSuffix(s, ".git")
	if i := strings.LastIndexAny(s, "/:\\"); i >= 0 {
		s = s[i+1:]
	}
	return s
}

// readMCP reads .mcp.json as a generic document, so a merge keeps every
// other key. A missing file is an empty document.
func readMCP(path string) (map[string]any, error) {
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

// keyIn is the Project Key in a .mcp.json document's code-central entry.
func keyIn(doc map[string]any) (string, bool) {
	servers, _ := doc["mcpServers"].(map[string]any)
	entry, _ := servers[serverName].(map[string]any)
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
		path := filepath.Join(d, mcpFile)
		if _, err := os.Stat(path); err == nil {
			doc, err := readMCP(path)
			if err != nil {
				return "", err
			}
			if k, ok := keyIn(doc); ok {
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

// writeMCP merges the code-central entry for key into doc and writes it.
func writeMCP(path string, doc map[string]any, key string) error {
	servers, ok := doc["mcpServers"].(map[string]any)
	if !ok {
		servers = map[string]any{}
		doc["mcpServers"] = servers
	}
	entry, ok := servers[serverName].(map[string]any)
	if !ok {
		entry = map[string]any{}
		servers[serverName] = entry
	}
	entry["type"] = "http"
	entry["url"] = mcpURL(key)
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}
