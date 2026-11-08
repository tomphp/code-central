# Setting up a Project

This guide sets a repository up as a Project, so that agents can reach its Specs and Tickets in Code Central. It follows a made-up repository, `acme-shop`, from an empty folder to a Claude Code session that lists the `code-central` MCP server. The output shown is copied from a real run, cut with `…` where it is long.

## Before you start

1. **Build and install `code-central`, and run the service.** Follow [Running Code Central as a service](running-as-a-service.md). Check that the service answers:

   ```console
   $ curl -sI http://localhost:7733/api/workflow
   HTTP/1.1 200 OK
   …
   X-Code-Central-Version: v1.…
   ```

2. **Install Claude Code and sign in.** Check that `claude` runs in a terminal.

3. **Install the `mattpocock-skills` plugin in Claude Code.** Each Stage runs one of its skills:

   ```console
   $ claude plugin install mattpocock-skills@claude-plugins-official
   ```

   Or run `/plugin` in a Claude Code session and install it from there. These are the skills an agent runs, as the Factory starts them:

   | Stage | Skill |
   | --- | --- |
   | grilling | `/grilling`, with `/domain-modeling` alongside |
   | spec | `/to-spec` |
   | tickets | `/to-tickets` |
   | implementation | `/implement` |

> [!WARNING]
> **Don't run `/setup-matt-pocock-skills` in a Project.** `code-central init` already writes `docs/agents/` and a pointer to it in `CLAUDE.md`, which tell the skills that Specs and Tickets live in Code Central. The setup skill would point them at GitHub issues or a `.scratch/` folder instead.

## A brand new repository

You're starting `acme-shop` from nothing, in `~/src`.

1. **Make the repository.**

   ```console
   $ cd ~/src
   $ mkdir acme-shop && cd acme-shop && git init
   Initialized empty Git repository in /Users/you/src/acme-shop/.git/
   ```

2. **Set it up as a Project.** Run `code-central init` and press Enter at both prompts to take the defaults:

   ```console
   $ code-central init
   Project Key [acme-shop]:
   Create Project acme-shop? [Y/n]:
   Created Project acme-shop.
   Project acme-shop is committed. Wrote .mcp.json, .claude/settings.json, docs/agents/issue-tracker.md, docs/agents/triage-labels.md, CLAUDE.md.
   Nothing was committed.
   Agents in a sandbox or container: see `code-central help sandboxes`.
   ```

   The Project Key it proposes is the repository's name (from its remote, or else the folder's name). It is permanent, so pick it now. `code-central init --key acme-shop` skips the first prompt.

   Check: `git status --short -uall` lists the files init wrote.

   ```console
   $ git status --short -uall
   ?? .claude/settings.json
   ?? .mcp.json
   ?? CLAUDE.md
   ?? docs/agents/issue-tracker.md
   ?? docs/agents/triage-labels.md
   ```

   These are the files and what each is for:

   | File | What it's for |
   | --- | --- |
   | `.mcp.json` | Adds the `code-central` MCP server, at `${CODE_CENTRAL_URL:-http://localhost:7733}/p/acme-shop/mcp`, so Claude Code can reach the Project. Any other servers in the file are kept. |
   | `.claude/settings.json` | The hooks: `code-central hook session-id` records the session on a Claim and a Round, and `code-central ping` marks the session active so its Claim doesn't turn Silent. |
   | `docs/agents/issue-tracker.md`, `docs/agents/triage-labels.md` | Tell the `mattpocock-skills` skills to use Code Central's operations, States and Labels instead of an issue tracker's. Re-running init rewrites them. |
   | `CLAUDE.md` | A short `## Issue tracker` section pointing at `docs/agents/issue-tracker.md`. An existing `CLAUDE.md` keeps its content; init only adds the section. |

3. **Commit the files.** init never commits. Committing them means every clone of `acme-shop` reaches the same Project.

   ```console
   $ git add . && git commit -m "Set acme-shop up as a Code Central Project"
   [main (root-commit) 7be68db] Set acme-shop up as a Code Central Project
    5 files changed, 73 insertions(+)
   …
   ```

4. **Check that an agent can reach the Project.** Start `claude` in `acme-shop`. If it asks whether you trust the folder, choose **Yes, I trust this folder**. It then asks about the new MCP server:

   ```text
   New MCP server found in this project: code-central
   MCP servers may execute code or access system resources. All tool calls
   require approval. Learn more in the MCP documentation.

     Use this MCP server
     Use this and all future MCP servers in this project
   ❯ Continue without using this MCP server
   ```

   Move up to **Use this MCP server** and press Enter. Then run `/mcp`: `code-central` is listed under the Project's `.mcp.json`, with a tick.

   ```text
   Manage MCP servers
   …
     Project MCPs (/Users/you/src/acme-shop/.mcp.json)
   ❯ ✔ code-central 32 tools
   …
   ```

   From the terminal, `claude mcp list` shows the same:

   ```console
   $ claude mcp list
   Checking MCP server health…
   …
   code-central: ${CODE_CENTRAL_URL}/p/acme-shop/mcp (HTTP) - ✔ Connected
   ```

   If it says `✘ Failed to connect`, the service isn't running: see step 1 of [Before you start](#before-you-start).

`acme-shop` is now a working Project. You can stop here and work it later, whichever way you choose: the [README](../README.md) lists the guides for working it.
