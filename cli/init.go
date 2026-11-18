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
	"github.com/tomoram/code-central/store"
)

const (
	mcpFile    = client.MCPFile
	serverName = client.ServerName
)

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
	localFlag := fs.Bool("local", false, "")
	committedFlag := fs.Bool("committed", false, "")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) > 0 {
		return usagef("unexpected argument %q", pos[0])
	}
	var wantMode store.Mode // no flag: follow the stored mode
	switch {
	case *localFlag && *committedFlag:
		return usagef("--local and --committed can't both be given")
	case *localFlag:
		wantMode = core.ModeLocal
	case *committedFlag:
		wantMode = core.ModeCommitted
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
	doc, err := client.ReadMCP(path)
	if err != nil {
		return err
	}
	proposed := ""
	if k, ok := client.KeyIn(doc); ok {
		switch p, err := c.GetProject(ctx, k); {
		case err == nil:
			// A known key: leave .mcp.json alone and refresh the rest.
			if p.Archived {
				return archivedProject(k)
			}
			fmt.Fprintf(e.Stdout, "%s already points at Project %s; refreshing the files init writes.\n", path, k)
			return setUp(ctx, e, ask, c, setup{root: root, inGit: inGit, key: k, project: p, wantMode: wantMode})
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
	var project core.ProjectView
	for {
		p, err := c.GetProject(ctx, key)
		if err == nil {
			if p.Archived {
				return archivedProject(key)
			}
			project = p
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
		created, err := c.CreateProject(ctx, core.CreateProjectInput{Key: key, Mode: wantMode})
		if err != nil {
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
		project = created
		fmt.Fprintf(e.Stdout, "Created Project %s.\n", key)
		break
	}

	if err := writeMCP(path, doc, key); err != nil {
		return err
	}
	return setUp(ctx, e, ask, c, setup{root: root, inGit: inGit, key: key, project: project, wantMode: wantMode, wroteMCP: true})
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

func serviceURL(e Env) string { return client.ServiceURL(e.Getenv) }

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

// archivedProject is init's answer for an Archived Project: it stays put
// away, and init writes nothing rather than quietly bringing it back.
func archivedProject(key string) error {
	return usagef("Project %s is archived, so init leaves it put away and writes nothing. "+
		"To work on it again, choose Unarchive in its row's menu in the Code Central UI, then run init again.", key)
}
