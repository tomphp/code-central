# 24: `init`, basic version

**What to build:** The human runs `code-central init` in a repository, accepts or overrides the proposed Project Key, and the repository is set up as a Project: the Project exists in the store and `.mcp.json` points agents at it. Shell commands run anywhere inside the repository find the same Project. See the [spec](../spec.md), section "Projects".

**Blocked by:** 22

**Status:** resolved

- [x] `init` proposes a key from the only remote, else `origin`, else the folder name: the repository name alone, lowercased; the human accepts or overrides it
- [x] A key the store already has is refused with a request for an override, unless the human confirms joining that existing Project
- [x] If the store doesn't know the key, `init` asks before creating the Project through `create_project`
- [x] `init` merges a `code-central` server entry at `${CODE_CENTRAL_URL:-http://localhost:7733}/p/<key>/mcp` into any existing `.mcp.json`, and never commits
- [x] If `.mcp.json` already holds a key the store knows, `init` leaves it alone
- [x] `init` works outside git
- [x] Shell commands find the Project from `--project`, then `CODE_CENTRAL_PROJECT`, then the key in the nearest `.mcp.json` above the current directory
- [x] How `init` derives the key is tested once in the CLI package against `codecentraltest`
