# Setting up a Project

This guide sets a repository up as a Project, so that agents can reach its Specs and Tickets in Code Central. It follows two made-up repositories to a Claude Code session that lists the `code-central` MCP server: `acme-shop`, started from an empty folder, and `acme-api`, which already has code and history. Then it shows where to choose how a Project's accepted work lands, and covers the cases where `init` behaves differently. The output shown is copied from a real run, cut with `…` where it is long.

## Before you start

1. **Install `code-central` and `code-central-factory`, and run the service.** Follow [Running Code Central as a service](running-as-a-service.md): download a release's archive for your OS and CPU and put both binaries on your PATH, or build them from source. Check that the service answers:

   ```console
   $ curl -sI http://localhost:7733/api/workflow
   HTTP/1.1 200 OK
   …
   X-Code-Central-Version: v1.…
   ```

2. **Install Claude Code and sign in.** Check that `claude` runs in a terminal. On Windows, the Factory needs the native `claude.exe`, installed with `irm https://claude.ai/install.ps1 | iex` in PowerShell: it refuses the npm shim `claude.cmd`, which can't carry a multi-line prompt.

3. **Install the `mattpocock-skills` plugin in Claude Code.** Each Stage runs one of its skills:

   ```console
   $ claude plugin install mattpocock-skills@claude-plugins-official
   ```

   Or run `/plugin` in a Claude Code session and install it from there. These are the skills an agent runs on each Stage's Ticket and on implementation Tickets, as the Factory starts them:

   | Ticket | Skill |
   | --- | --- |
   | grilling Stage | `/grilling`, with `/domain-modeling` alongside |
   | spec Stage | `/to-spec` |
   | tickets Stage | `/to-tickets` |
   | implementation | `/implement` |
   | a Map's Charting (`wayfinder:chart`) | `/wayfinder` |
   | a Map's grilling (`wayfinder:grilling`) | `/grilling`, with `/domain-modeling` alongside |
   | a Map's task (`wayfinder:task`) | none: the Guide alone |
   | a Map's research (`wayfinder:research`) | `/research`, on a local `research/<ticket>` branch |
   | a Prototype (`prototype`) | `/prototype`, on a local `prototype/<ticket>` branch |

   The grilling Stage, Charting and a Map's grilling Ticket hand a Question answered **Prototype this** to a new Prototype, then pick up again as a redo once it is Done: see [Prototypes](working-with-claude-code.md#prototypes).

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

   The Project Key it proposes is the repository's name (from its remote, or else the folder's name). It is permanent, so pick it now. `committed` is the Project's mode: init's files are meant to be committed to the repository, as in step 3. `code-central init --key acme-shop` skips the first prompt.

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
   | `docs/agents/issue-tracker.md`, `docs/agents/triage-labels.md` | Tell the `mattpocock-skills` skills to use Code Central's operations, States and Labels instead of an issue tracker's. Each starts with a `Written by code-central init` header line, and re-running init rewrites a file that has it. Take the header line out to keep your own version: init then asks before replacing it. |
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
   ❯ ✔ code-central 33 tools
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

`acme-shop` is now a working Project. You can stop here and work it later, whichever way you choose: the guides after this one in the [README](../README.md)'s list cover working it.

## An existing repository

`acme-api` is a repository you already have in `~/src`, with code, history and a `CLAUDE.md`. It also has a small `.scratch/` folder from planning: one spec and two tickets. Do [Before you start](#before-you-start) first, if you haven't.

1. **Start from a clean working tree.** Commit or stash what you're working on, so that afterwards `git status` shows only what init changed.

   ```console
   $ cd ~/src/acme-api
   $ git status --short
   $ git log --oneline
   87bd9af Plan rate limits
   59d7d06 Health endpoint
   ```

2. **Set it up as a Project.** Run `code-central init` and press Enter at both prompts:

   ```console
   $ code-central init
   Project Key [acme-api]:
   Create Project acme-api? [Y/n]:
   Created Project acme-api.
   Project acme-api is committed. Wrote .mcp.json, .claude/settings.json, docs/agents/issue-tracker.md, docs/agents/triage-labels.md, CLAUDE.md.
   Nothing was committed.
   Agents in a sandbox or container: see `code-central help sandboxes`.
   ```

   `acme-api` has a remote, `git@github.com:acme/acme-api.git`, so the Project Key init proposes is the repository's name from it.

   Check: `git status --short -uall` lists the same files as for a brand new repository (see the table in [A brand new repository](#a-brand-new-repository)), except that `CLAUDE.md` is changed rather than new:

   ```console
   $ git status --short -uall
    M CLAUDE.md
   ?? .claude/settings.json
   ?? .mcp.json
   ?? docs/agents/issue-tracker.md
   ?? docs/agents/triage-labels.md
   $ git diff CLAUDE.md
   …
    # acme-api

    Run `go test ./...` before committing.
   +
   +## Issue tracker
   +
   +Specs and Tickets live in Code Central: see `docs/agents/issue-tracker.md`.
   ```

   init leaves everything else alone, `.scratch/` included. Leave `.scratch/` where it is for now: [Importing `.scratch/` into a Project](importing-scratch.md) brings it into the Project.

3. **Commit the files.**

   ```console
   $ git add . && git commit -m "Set acme-api up as a Code Central Project"
   [main 7990c97] Set acme-api up as a Code Central Project
    5 files changed, 74 insertions(+)
   …
   ```

4. **Check that an agent can reach the Project**, as in step 4 of [A brand new repository](#a-brand-new-repository): start `claude` in `acme-api`, choose **Use this MCP server**, and run `/mcp`. Until you've done that, `claude mcp list` shows the server waiting for you:

   ```console
   $ claude mcp list
   …
   code-central: ${CODE_CENTRAL_URL}/p/acme-api/mcp (HTTP) - ⏸ Pending approval (run `claude` to approve)
   ```

   After you approve it, the line ends `✔ Connected`.

`acme-api` is now a working Project.

## Choosing how accepted work lands

Each Project has a **Landing Method**: what the Factory does with an implementation Ticket's branch once you accept it. A new Project starts on **Merge into base** with **Push** off: the Factory merges the branch into the base locally and lands the Ticket, and nothing leaves the machine. Leave it there unless you want something else.

To change it, open the UI, click **⋯** beside the Project in the left pane, and choose **Options**. The Project Options page has its own address, `/?view=project-options&project=acme-api`, so you can bookmark it. Its **Landing Method** section offers:

- **Merge into base**: the Factory merges the accepted branch into the base and lands the Ticket. The default.
- **Leave as a branch**: the Factory leaves the branch for you to merge, and lands the Ticket when it sees the merge in the base.
- **Push**, with a **Remote** (`origin` unless you change it): under Merge, the Factory pushes the base to the remote after merging, and the Ticket lands only once the push succeeds; under Leave as a branch, it pushes the Ticket's branch when you accept it. The Factory checks the remote exists, never force-pushes, and under Merge pushes the whole base, your own commits on it too. Remote can't be empty while Push is on: Save says so.

**Save** applies the change to the Project in Code Central, and every open UI tab shows it. An Archived Project has no Options item, and its Project Options page is read-only until you unarchive it.

## Watching CI

**CI Tracking** has Code Central watch the CI of what the Factory pushes. It is off on every Project until you turn it on, and Agents have no tool or command to change it. Turn it on in the **CI** section of the same Project Options page:

- **CI Tracking**: on or off.
- **CI Provider**: **GitHub Actions**, the only one for now. Code Central uses your existing `gh` login and stores no GitHub token.
- **GitHub repository**: the repository to watch, as `owner/name`, such as `acme/acme-api`. CI Tracking can't be on without one: Save says so, as it does for anything that isn't `owner/name`. When the Factory starts, it reports the URL of the Push remote (the Landing Method's Remote, `origin` unless you changed it), leaving out any login an HTTPS URL carries. If that URL is GitHub's, over HTTPS or SSH, the field is filled in from it, with a note naming the URL; change it if CI runs somewhere else, such as a fork.
- **Ignored workflows**: workflow file names, such as `nightly.yml, preview.yml`, separated by commas. Their runs won't count towards a Ticket's CI result. Name the file, not its path: `ci.yml`, not `.github/workflows/ci.yml`.

**Save** applies the settings, and every open UI tab shows them. Turning CI Tracking off keeps the repository and Ignored workflows for the next time you turn it on. For now the CI section only records these settings: the Factory lands accepted work the same way with CI Tracking on or off.

### CI Rules

A **CI Rule** says "when this workflow fails on this branch, open a Ticket to fix it", for failures no Ticket's own CI catches, such as a red `main` after a hand merge. Add, edit and remove them under **CI Rules**, at the foot of the CI section. Agents have no tool or command to change them. A rule names:

- **Workflow**: one workflow file name, such as `ci.yml`, or `any` for every workflow but the Ignored ones.
- **Branch**: an exact branch name, such as `main`. Globs such as `release/*` are refused.
- **Tickets open as**: **Draft**, for you to look at first, or **Ready**, for the Factory to start fixing straight away.

**Add rule** creates the rule and its own Spec, titled after what it watches, such as `CI: ci.yml on main` or `CI: any workflow on main`. The rule's Tickets will go on that Spec, so its fix Tickets stay together ([ADR 0017](adr/0017-a-ci-rule-opens-its-tickets-on-a-long-lived-spec-of-its-own.md)). Two rules can't watch the same workflow on the same branch. Each rule is listed with its Spec's ID:

```
ci.yml on main: opens Ready Tickets on acme-api-42      Edit  Remove
```

**Edit** changes a rule in place, and its Spec's title changes to match unless you renamed the Spec. **Remove** removes the rule and keeps its Spec as the record of its Tickets. For now rules are only recorded: none opens a Ticket yet.

## When `init` behaves differently

### A repository you don't own: `init --local`

`widgets` is someone else's open source repository, cloned from `https://github.com/someone/widgets.git`. You want to track your work on it in Code Central without committing init's files to their repository. `--local` keeps them out of git.

1. **Set it up as a local Project.**

   ```console
   $ cd ~/src/widgets
   $ code-central init --local
   Project Key [widgets]:
   Create Project widgets? [Y/n]:
   Created Project widgets.
   Project widgets is local. Wrote .mcp.json, .claude/settings.local.json, docs/agents/issue-tracker.md, docs/agents/triage-labels.md, CLAUDE.local.md.
   They are listed in .git/info/exclude; nothing was committed.
   Agents in a sandbox or container: see `code-central help sandboxes`.
   ```

   `local` is now the Project's mode. The hooks go in `.claude/settings.local.json` and the pointer in `CLAUDE.local.md`, both of which Claude Code reads but which aren't meant to be shared. `widgets`' own `CLAUDE.md` is left alone.

2. **Check that git doesn't see them.** `git status` is clean, and `.git/info/exclude` lists init's files under its mark:

   ```console
   $ git status --short -uall
   $ tail -6 .git/info/exclude
   # code-central init --local
   /.mcp.json
   /.claude/settings.local.json
   /CLAUDE.local.md
   /docs/agents/issue-tracker.md
   /docs/agents/triage-labels.md
   ```

3. **Check that an agent can reach the Project**, as in step 4 of [A brand new repository](#a-brand-new-repository).

The mode belongs to the Project, not the clone. If you later want init's files committed after all, `code-central init --committed` asks before switching the Project, then writes the committed files and takes its lines out of `.git/info/exclude`. `CLAUDE.local.md` and `.claude/settings.local.json` stay behind as untracked files: delete them or leave them out of the commit.

### A second clone or worktree

A Project is one repository, however many clones or worktrees of it you have. Run `code-central init` in each one, and answer so that it joins the Project you already have rather than creating a second one. What init does depends on whether the clone already has `.mcp.json`.

**When `.mcp.json` is committed**, as in `acme-api`, every clone and worktree already points at the Project. init says so and refreshes the files it writes, without asking anything:

```console
$ cd ~/src/acme-api
$ git worktree add ../acme-api-rate-limits -b rate-limits
…
$ cd ../acme-api-rate-limits
$ code-central init
/Users/you/src/acme-api-rate-limits/.mcp.json already points at Project acme-api; refreshing the files init writes.
Project acme-api is committed. Wrote .claude/settings.json, docs/agents/issue-tracker.md, docs/agents/triage-labels.md.
Nothing was committed.
Agents in a sandbox or container: see `code-central help sandboxes`.
$ git status --short
```

The files it refreshed were the same as the committed ones, so `git status` is clean.

**When `.mcp.json` isn't committed**, as in a local Project like `widgets`, a new clone has no `.mcp.json`. init proposes the same Project Key, finds that a Project already has that Project Key, and asks whether to join it:

```console
$ git clone https://github.com/someone/widgets.git ~/src/widgets-2
…
$ cd ~/src/widgets-2
$ code-central init
Project Key [widgets]:
Project widgets already exists. Join it? Only if this repository is that Project. [y/N]: y
Project widgets is local. Wrote .mcp.json, .claude/settings.local.json, docs/agents/issue-tracker.md, docs/agents/triage-labels.md, CLAUDE.local.md.
They are listed in .git/info/exclude; nothing was committed.
Agents in a sandbox or container: see `code-central help sandboxes`.
```

Answer `y` when this clone is the same repository as the Project: its work is the Project's work. init takes the Project's mode (here `local`) without being told. Answer `n`, or press Enter, when it's a different repository that happens to have the same name: init asks you to choose another Project Key, then asks before creating a new Project with it.

Check: `git status --short -uall` is clean, as in `widgets`:

```console
$ git status --short -uall
```

### Re-running init after an upgrade

After you upgrade Code Central (see [Running Code Central as a service](running-as-a-service.md)), run `code-central init` again in each Project, so its files match the new version. It never changes `.mcp.json` or the Project Key.

In `acme-api`, someone has replaced `docs/agents/triage-labels.md` with their own version: they took out its header line and added a line at the end. Code Central has been upgraded from v1.0.0 to v1.1.0.

1. **Re-run init.** It rewrites the files it wrote before. For the one you've changed, it shows how yours differs from its own and asks before replacing it. Press Enter to keep yours:

   ```console
   $ cd ~/src/acme-api
   $ code-central init
   /Users/you/src/acme-api/.mcp.json already points at Project acme-api; refreshing the files init writes.
   docs/agents/triage-labels.md exists and wasn't written by init:
   --- docs/agents/triage-labels.md (yours)
   +++ docs/agents/triage-labels.md (Code Central's)
   +<!-- Written by code-central init v1.1.0. Re-running init rewrites this file. -->
   +
    # Triage labels
   …
    When a skill applies a role that is a State, use the operation beside it instead of adding a Label.
   -
   -We triage on Mondays.
   Replace docs/agents/triage-labels.md? [y/N]:
   Project acme-api is committed. Wrote .claude/settings.json, docs/agents/issue-tracker.md.
   Kept your docs/agents/triage-labels.md.
   Nothing was committed.
   Agents in a sandbox or container: see `code-central help sandboxes`.
   ```

   Answer `y` to take Code Central's version instead; it then appears in the `Wrote` line. A file that still starts with init's header line is rewritten without asking, so an edit you make under the header is lost the next time init runs.

2. **Check what changed, and commit it.** Here only the version in the header changed:

   ```console
   $ git status --short
    M docs/agents/issue-tracker.md
   $ git diff
   …
   -<!-- Written by code-central init v1.0.0. Re-running init rewrites this file. -->
   +<!-- Written by code-central init v1.1.0. Re-running init rewrites this file. -->
   …
   $ git commit -am "Refresh Code Central's files for v1.1.0"
   [main 6d93dd2] Refresh Code Central's files for v1.1.0
    1 file changed, 1 insertion(+), 1 deletion(-)
   ```

   In a local Project there is nothing to commit: init's files are listed in `.git/info/exclude`.

### Agents in a sandbox or container

The service listens only on your machine's loopback, so an agent in a sandbox or container needs a way through to it. Every `init` ends by pointing at the help topic that covers this; read it there:

```console
$ code-central help sandboxes
```
