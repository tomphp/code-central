# How a Project is identified

Type: grilling
Status: resolved
Blocked by: 03

## Question

How is a Project added to Code Central, and what identifies it? Given an agent in a worktree, a clone or a sandbox, how is it matched to the right Project?

## Answer

Amended by [How finished work is kept and hidden](20-finished-work.md), which added Archived Projects: `archive_project` and `unarchive_project`, refused while any Ticket is unfinished, and `init` saying a Project is archived.

Amended by [How agent sessions are taught to use Code Central](17-teaching-agent-sessions.md): with the human's confirmation, `init` can join a key that already exists, and `init --local` sets a Project up without committing `.mcp.json`.

A Project is identified by a Project Key: a short, unique, permanent name committed to the repository. An agent identifies itself as the Project by connecting with that key, so nothing is worked out from where the agent happens to be running.

**The key is derived once, at setup.** Running `code-central init` proposes a key, and the human accepts or overrides it. The proposal comes from the only remote if there is one, otherwise from `origin`, otherwise from the folder name. It is the repository name alone, lowercased. If the key is taken, `init` refuses and asks for an override. The key can't be changed afterwards, but the Project's display name can.

**The key travels in the MCP URL.** `init` writes it into the committed `.mcp.json`, as in `${CODE_CENTRAL_URL:-http://localhost:<port>}/p/code-central/mcp`. A connection belongs to one Project, and MCP tools take no Project argument. An HTTP MCP server has no documented way to see a session's working directory, so the key has to be stated somewhere. A sandbox sets `CODE_CENTRAL_URL` to reach the host, and the committed file stays the same.

**Shell commands use the same copy.** They look for the `--project` flag, then `CODE_CENTRAL_PROJECT`, then the key in the nearest `.mcp.json` above the current directory. There is no second marker file.

**A Project is one logical repository.** Clones and worktrees are the same Project because they carry the same committed file. A fork stays the same Project until someone runs `init` with a new key. A monorepo is one Project, with Specs or Labels separating its products.

**Adding a Project.** `init` is the only way to add one in v1, and it is a deliberate act by the human. Agents can't create Projects, and nothing is created automatically, so naming an unknown key is a clear error. `init` merges into an existing `.mcp.json`, doesn't commit, and works outside git. If the file's key already exists, `init` does nothing. If the store doesn't know the key, it asks before creating that Project. The UI lists and renames Projects but doesn't create them, because it has no way to write to a repository.

**What a Project stores.** Only its key and display name in v1, with no path and no remote. An Import takes its folder as an argument. There is no way to delete a Project in v1, and archiving stays with the Ticket history fog.

**Known limitation.** Committing `.mcp.json` doesn't suit a shared repository: other people get a server that fails to connect, and some repositories can't take a file for this tool at all. v1 accepts that for simplicity, but it needs resolving later, because Code Central is wanted in repositories like that. No ADR was written, because this is expected to change.

Considered and rejected: identifying a Project by its path, because every copy sees a different one. Deriving the key on every connection from the remote or folder, because a worktree or sandbox with no remote gets the wrong key, a rename silently loses the Project, and an override would have to be repeated in every copy. `acme/api` as the key's form, because it adds a slash everywhere to avoid a collision `init` already catches. A static header or a tool argument for the key, because a wrong argument files a Ticket under the wrong Project with no error. A marker file alongside `.mcp.json`, because two copies can disagree. Creating Projects in the UI, because the result would have no committed key and nothing could reach it.

Not verified: whether MCP `roots` works over HTTP in Claude Code, though it is deprecated in the 2026-07-28 spec either way, and whether `.mcp.json` approval is remembered per repository, worktree or clone. Interactive sessions prompt for approval, and `claude -p` and SDK sessions load the server without asking.
