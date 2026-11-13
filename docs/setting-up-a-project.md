# Setting up a Project

This guide sets a repository up as a Project, so that agents can reach its Specs and Tickets in Code Central. It follows two made-up repositories to a Claude Code session that lists the `code-central` MCP server: `acme-shop`, started from an empty folder, and `acme-api`, which already has code and history. Then it shows where to choose how a Project's accepted work lands and which models its sessions run, and covers the cases where `init` behaves differently. The output shown is copied from a real run, cut with `…` where it is long.

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
   | tickets Stage (skipped when you choose **Implement** on the spec Stage's review) | `/to-tickets` |
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

Each Project has a **Landing Method**: what the Factory does with an implementation Ticket's branch once you accept it. A new Project starts on **Merge** with **Push** off: the Factory merges the branch into the base locally and lands the Ticket, and nothing leaves the machine. Leave it there unless you want something else.

To change it, open the UI, click **⋯** beside the Project in the left pane, and choose **Options**. The Project Options page has its own address, `/?view=project-options&project=acme-api`, so you can bookmark it. On a wide screen a menu on the left lists the page's sections (Landing Method, CI, GitHub Issues, Models, Stage Guides and Auto-accept) and stays in view as you scroll; choosing an entry jumps to that section. On a narrow screen the menu is hidden. Its **Landing Method** section starts with the **Base branch**, a text field showing the branch the Project's work is cut from and lands on (`main` until you change it; it is saved with the rest of the section, and Save refuses a name git would not accept). Below it are the three methods:

- **Merge**: the Factory merges the accepted branch into the base and lands the Ticket. The default.
- **Leave as a branch**: the Factory leaves the branch for you to merge, and lands the Ticket when it sees the merge in the base.
- **Pull Request**: the Code Host is worked out from the Push remote's URL, which the Factory reports when it starts, so start the Factory once first. On GitHub, a Pull Request Watcher opens a pull request into the base for each pushed branch, through your `gh` login, which needs write access to the repository. Turning it on checks that, and Save refuses with the reason and fix when `gh` is missing, logged out or can't write to the repository. On Azure Repos the Watcher uses the PAT in the **PAT variable** (`ADO_PAT` by default; it needs Code: Read & write), and Save refuses when the variable is unset or empty, the repository can't be read or the PAT can't contribute to pull requests. The Watcher's status, Watching or an error with its fix, shows under the Landing Method options, and an error sends one Notification. [The Pull Request Watcher](running-as-a-service.md#the-pull-request-watcher) in Running as a service describes it. Switching away from Pull Request: waiting Tickets follow the new method on the Factory's next poll (under Merge a Ticket already in the base lands and the others merge; under Branch they wait for their branch in the base), the Watcher stops, and open pull requests stay on the host. Each Accepted Ticket with an open one gets a comment, and the Landing Method section lists how many remain open, with links, so you can close them yourself.
- **Land per Spec** (under Merge, on by default): the implementation Tickets of every Spec but a Long-lived one are first merged into a shared Spec Branch, `spec/<spec id>`, as you accept them, and the Factory merges that branch into the base and lands the Tickets together once every Ticket on the Spec is accepted (a Requirement's tickets Stage must be finished too). A Ticket blocked by another Ticket on the same Spec starts as soon as that one is accepted, without waiting for it to land. The Ticket shows a **Waiting for the Spec** badge until then. Turn it off to land each Ticket as you accept it, as Merge always did; a Long-lived Spec, one labelled `long-lived` such as a CI Rule's or the Issues Spec, always lands Ticket by Ticket. Anyone can add or remove the `long-lived` Label on a Spec. Under Leave as a branch it has no effect. [Land per Spec](running-the-factory.md#land-per-spec) in Running the Factory shows the Factory's output.
- **Push**, with a **Remote** (`origin` unless you change it): under Merge, the Factory pushes the base to the remote after merging, and the Ticket lands only once the push succeeds; under Leave as a branch, it pushes the Ticket's branch when you accept it. The Factory checks the remote exists, never force-pushes, and under Merge pushes the whole base, your own commits on it too. Remote can't be empty while Push is on: Save says so.

**Save** applies the change to the Project in Code Central, and every open UI tab shows it. An Archived Project has no Options item, and its Project Options page is read-only until you unarchive it.

### Pull Request: the PAT, its scopes and a diverged base

Pull Request needs the Factory to have started once, so that Code Central knows the Push remote's URL and works the Code Host out from it: github.com is GitHub; dev.azure.com, ssh.dev.azure.com and `*.visualstudio.com` are Azure Repos. The Landing Method section shows the host and repository, and doesn't let you edit them. Nothing depends on CI Tracking.

- **GitHub** uses your `gh` login, which needs write access to the repository (pull requests are opened through it). Nothing else is stored.
- **Azure Repos** uses a PAT that the service's environment holds, named by the **PAT variable** field (`ADO_PAT` unless you name another). Make it in Azure DevOps (**User settings**, **Personal access tokens**) with the scope **Code: Read & write**. If you also watch Azure Pipelines, the same variable can serve both when the token has both scopes, **Build: Read & execute** and **Code: Read & write**; each check looks only for its own. [Giving the service an environment variable](running-as-a-service.md#giving-the-service-an-environment-variable) shows how to set it.

Turning Pull Request on runs the check once and creates no pull request. Save refuses with the reason and fix when there is no Push remote URL yet, it names no supported host, `gh` is missing, logged out or can't write, or on Azure Repos the variable is unset or empty, the repository can't be read or the PAT can't contribute to pull requests. For example, with the variable unset on macOS:

```text
The environment variable ADO_PAT is not set, or is empty, in the Pull Request Watcher's environment.
Make an Azure DevOps PAT with Code: Read & write, run `launchctl setenv ADO_PAT <PAT>`, then restart the Code Central service.
```

Under Pull Request there is no CI gate and **Land anyway** isn't offered.

**Base diverged.** Under Pull Request, and under Merge with Push on, the Factory keeps your local base level with the remote's by fetching and fast-forwarding it. Under Pull Request, if the local base holds commits the remote's doesn't, it can't. Under Merge + Push, a local base that is only ahead of the remote's is current, since the Factory merges locally and then pushes; when the remote holds commits the local base lacks as well, the Factory moves the local base onto the remote's, rebuilding its own unpushed merges and replaying your unpushed commits, and it can't only when one of those conflicts or an unpushed merge isn't its own. Either way Code Central shows a **Base diverged** item in Needs Me and Focus Mode, one per Project, with the two commits. Until you reconcile them by hand, for example by resetting or rebasing your local base onto the remote's, the Factory lands nothing and claims no work that would cut a branch. You do nothing in Code Central: on its next poll the Factory sees the fix and carries on. [Pull Request](running-the-factory.md#pull-request) in Running the Factory shows its lines.

**Base missing.** Under every Landing Method, if the Project's base branch exists neither locally nor on the Push remote, the Factory reports it and Code Central shows a **Base missing** item in Needs Me and Focus Mode, one per Project, naming the branch and linking to Project Options. Create the branch, or change the Base branch in Project Options. The item clears on the Factory's next poll that finds the branch. It never shows beside Base diverged, which is a Pull Request and Merge + Push item.

## Watching CI

**CI Tracking** has Code Central watch the CI of what the Factory pushes. It is off on every Project until you turn it on, and Agents have no tool or command to change it. Turn it on in the **CI** section of the same Project Options page:

- **CI Tracking**: on or off.
- **CI Provider**: **GitHub Actions** or **Azure Pipelines**. GitHub Actions uses your existing `gh` login and stores no GitHub token. Azure Pipelines calls Azure DevOps' API with a PAT that the service's environment holds; see [Azure Pipelines](#azure-pipelines) below. Each provider keeps its own fields, so switching the CI Provider loses nothing.
- **GitHub repository** (GitHub Actions): the repository to watch, as `owner/name`, such as `acme/acme-api`. CI Tracking can't be on without one: Save says so, as it does for anything that isn't `owner/name`. When the Factory starts, it reports the URL of the Push remote (the Landing Method's Remote, `origin` unless you changed it), leaving out any login an HTTPS URL carries. If that URL is GitHub's, over HTTPS or SSH, the field is filled in from it, with a note naming the URL; change it if CI runs somewhere else, such as a fork.
- **Azure DevOps project URL** and **PAT environment variable** (Azure Pipelines): see [Azure Pipelines](#azure-pipelines) below.
- **Ignored workflows**: workflow names, separated by commas. Their runs won't count towards a Ticket's CI result. For GitHub Actions a workflow is its file name, such as `nightly.yml, preview.yml`: name the file, not its path, `ci.yml`, not `.github/workflows/ci.yml`. For Azure Pipelines it is the pipeline's name, such as `Nightly build, Deploy preview`, spaces allowed, without its folder. Changing the CI Provider leaves the list as it is; names that no longer match anything match nothing.

**Save** applies the settings, and every open UI tab shows them. Turning CI Tracking on is checked first: Code Central checks that `gh`, the GitHub CLI, is installed and logged in, and can read the repository. If any of these fails, Save refuses, and the CI section says what is wrong and how to fix it, for example:

```text
gh isn't logged in to GitHub: You are not logged into any GitHub hosts. To log in, run: gh auth login
Run `gh auth login` as the user the service runs as.
```

[Finding `gh`](running-as-a-service.md#finding-gh) says where Code Central looks for `gh`, and how the **gh path** Option points it somewhere else.

### Azure Pipelines

Choose **Azure Pipelines** as the CI Provider to watch Azure DevOps' CI. Code Central calls Azure DevOps' REST API itself, from the CI Watcher, and stores no PAT, only the name of the environment variable that holds it.

- **Azure DevOps project URL**: the one project to watch, as its URL, such as `https://dev.azure.com/acme/web`. CI Tracking can't be on without one. Paste what the browser shows: any `/_build`, `/_git/repo` or `/_settings` tail, query, fragment, login and trailing slash are dropped. The forms are:
  - Azure DevOps Services: `https://dev.azure.com/<organization>/<project>`, or `https://<organization>.visualstudio.com/<project>` (with or without `/DefaultCollection`).
  - Azure DevOps Server: `https://<host>/<collection>/<project>`, such as `https://tfs.example.com/tfs/DefaultCollection/web`. `http://` is accepted for a Server on an intranet.

  A URL that doesn't parse is refused on the field. When the Push remote is an Azure Repos one over HTTPS (`https://dev.azure.com/acme/web/_git/web`), or SSH on Azure DevOps Services (`git@ssh.dev.azure.com:v3/acme/web/web`), and no project URL is stored, the field is filled in from it, with a note naming the remote. Server SSH remotes aren't filled in.
- **PAT environment variable**: the name of the environment variable that holds the PAT, `ADO_PAT` unless you name another: a letter or underscore, then letters, digits and underscores.

Make a PAT in Azure DevOps (**User settings**, **Personal access tokens**) with the scope **Build: Read & execute**. Read lists runs and jobs; execute is what **Re-run failed jobs** needs. Code Central can't confirm the execute scope without retrying a run, so a turned-on check doesn't, and a re-run Azure DevOps refuses for scope shows as a refused Re-run request. Then put the PAT in the variable in the *service's* environment, since your shell's isn't passed on: [Giving the service an environment variable](running-as-a-service.md#giving-the-service-an-environment-variable) shows how on each OS. Nothing is captured when you `service install`.

Turning CI Tracking on is checked first, and Save refuses with the reason and its fix when the variable is unset or empty, when Azure DevOps rejects the PAT (expired or revoked), when it can't read the project (refused on the project URL field), or when the PAT lacks Build: Read. For example, with the variable unset on macOS:

```text
The environment variable ADO_PAT is not set, or is empty, in the CI Watcher's environment.
Make an Azure DevOps PAT with Build: Read & execute, run `launchctl setenv ADO_PAT <PAT>`, then restart the Code Central service.
```

Everything else works as for GitHub Actions, with these differences:

- A run's **Workflow** is its pipeline's name, without its folder, and its name is the build number. Ignored workflows and CI Rules match pipeline names.
- Runs are matched to a Ticket by its pushed commit, whatever repository in the project they built. Only the project's latest 200 builds are searched, which is plenty for a commit that is still waiting.
- A run's jobs are the build's jobs; stages and tasks aren't listed.
- A run that **partially succeeded** counts as **passed**, and its row says **Partially succeeded** with a warning mark.
- **Re-run failed jobs** asks Azure DevOps to retry the build's failed jobs, as a new attempt of the same build.
- Azure DevOps timing a job out reports it as failed or cancelled, and Code Central takes that as given.
- Links read **View on Azure DevOps**.
- Throttling by Azure DevOps (HTTP 429, or 503 with `Retry-After`) is handled like GitHub's rate limit: Watching, with the time it calls again.

While CI Tracking is on, the CI section shows the **CI Watcher**'s status below Save. The CI Watcher is the process that watches the Project's CI. On every check it makes sure again that `gh` is installed and logged in, and can read the repository, then asks GitHub about the runs of each pushed commit still waiting on CI. It checks every 30 seconds while a pushed commit waits, and every 2 minutes otherwise:

- **Watching acme/acme-api.**: it can reach GitHub. With Azure Pipelines it reads **Watching https://dev.azure.com/acme/web.**
- **Watching acme/acme-api.** with **GitHub is rate-limiting it until 10:15**: GitHub has refused its calls for its rate limit. It makes no calls until the time GitHub gave, then carries on by itself. When GitHub gives no time, it waits twice as long as it last did, up to 10 minutes. This isn't an error, and sends no Notification.
- **Error**: it can't. The status says what is wrong and how to fix it, as a refused Save does. You get one Notification when it goes into an error, not one per check. Once you fix the cause, the next check brings it back to Watching by itself, with nothing to restart. Changing the **gh path** Option counts too: the CI Watcher reads it on every check.
- **Error** saying **The CI Watcher crashed, restarted 3 times**: the CI Watcher process keeps exiting. The service reports this one, with the last line the CI Watcher wrote, and restarts it by itself, waiting longer after each crash, up to 5 minutes. Its output is in [the service log](running-as-a-service.md#the-service-log). It stays shown until the CI Watcher has run for 10 minutes without crashing, and you get one Notification when it starts crashing, not one per restart.
- **Starting; it hasn't reported yet.**: the CI Watcher hasn't checked since the service started or CI Tracking was turned on. The service starts it with itself and when you turn CI Tracking on, and it checks straight away, so this lasts moments. Changing the repository starts it here again.

Turning CI Tracking off keeps the repository and Ignored workflows for the next time you turn it on.

While CI Tracking and Push are both on, each push the Factory makes for a Ticket is tracked: under **Merge + Push**, the base it pushes after merging the Ticket; under **Branch + Push**, the Ticket's branch, pushed when you accept it. That commit is the Ticket's **pushed commit**. Only pushes made while CI Tracking is on are tracked. The CI Watcher finds the workflow runs GitHub started for it, and the Ticket's panel gains a **CI** section once it has any: the Ticket's **CI result**, then each workflow run with its status, its jobs and links to them on GitHub. The CI result is:

- **running** while any of the pushed commit's runs hasn't finished, or before any has started;
- **failed** if any run failed or timed out;
- **cancelled** if none failed or timed out but any run was cancelled, say by you or by GitHub. A cancelled run is no failure, so it shows in neutral grey, labelled **cancelled**, where a failed run is red;
- **passed** otherwise. A skipped or neutral run counts as passed, and so does an Azure Pipelines run that partially succeeded.

Runs of Ignored workflows are listed greyed and marked **ignored**, and never count towards the CI result. When a Ticket is pushed again, say after it is sent back and accepted again, the newer push gives the CI result, and the earlier push's runs are kept under **Earlier pushes**.

A Ticket with a CI result also shows it as a badge, **CI running**, **CI passed**, **CI failed** or **CI cancelled**, on its row in the Spec view, on its card on the Board and its row in the Inbox's Ticket lists (both leave it off a Done Ticket), on its commit's item in Needs Me and in the Activity Dashboard. The Activity Dashboard's **CI Running** block also counts the CI Runs still going that count towards a Ticket's CI result, and opens the CI view; its CI Running list below the In Progress Tickets lists those runs, each with its GitHub link. Above its Tickets, the Spec view sums their badges, such as **1 failed**, **1 cancelled** and **3 passed**, so you can see a Spec's CI health at a glance.

Under **Merge + Push**, CI Tracking gates landing: the Factory pushes an accepted Ticket and leaves it Accepted until its CI result is passed, then lands it, so Done means on the remote and green. A later commit on the base that contains the Ticket's work, such as another Ticket's merge or a commit you push by hand, becomes the Ticket's newest pushed commit too, and lands it when it passes. A Ticket it blocks waits until it lands. A failed CI result puts the pushed commit in Needs Me as **CI failed**, one item listing every Ticket waiting on it, where you can **Re-run failed jobs**, **Land anyway**, open a **New fix Ticket** (on the Project's CI Fixes Spec, leaving them waiting for the fix to pass), or, when it lists one Ticket alone, **Send back to fix**; your choice acts on all of them. A cancelled one puts it there as **CI cancelled**, offering the same options except New fix Ticket. Under **Branch + Push** the CI result only informs you, though you can still re-run the failed jobs or fix it before you merge, and the Factory lands the Ticket when it sees your merge. [With CI Tracking](running-the-factory.md#with-ci-tracking) in Running the Factory shows how.

### The CI view

**CI**, in the **Admin** section of the Inbox's left pane below **Usage**, opens the CI view on the Project chosen in the left pane, or on **All Projects** when none is. Its **Projects** picker changes that, and goes in the address (`/?view=ci&project=acme`); **Back to the Inbox** returns you to where you were. All Projects covers every active Project with CI Tracking on.

At the top, each Project's CI Watcher status: Watching its repository (or Azure DevOps project URL), Starting, or an error with its fix, as in the CI section; a Project with CI Tracking off says so, linking its Project Options. Below it, the CI Runs: those still running first, newest first, then those that finished in the last 7 days, the latest to finish first. Each row has the workflow's name and file, branch, short commit, status, how long it ran (so far, while running), the Ticket or CI Rule it is tracked against, which opens it, and **View on GitHub** (**View on Azure DevOps** for an Azure Pipelines Project). A run that partially succeeded shows as **Partially succeeded**, in the passed colour with a warning mark. A re-run shows its latest attempt. Runs of Ignored workflows are greyed and marked **ignored**. The view follows the live feed: a run that changes, or a new one, pulses.

```
Acme API: Watching acme/acme-api.

Workflow        Branch   Commit    Status    Duration  Tracked against   GitHub
CI ci.yml       main     3f9c2a1   running   2m 40s    acme-api-17       View on GitHub
Lint lint.yml   main     3f9c2a1   passed    48s       acme-api-17       View on GitHub
```

Code Central keeps each CI Run for 30 days after it finishes, then prunes it. A Ticket whose runs have all been pruned keeps its pushed commit and has no CI result.

Scripts can read the CI view over the HTTP API: `GET /api/ci?project=<key>`, or leave out `project` for All Projects. An unknown Project is `not_found`. The answer's `projects` are the CI Watchers' statuses, each with its `project`, `tracking`, `repository` and `watcher`, and `runs` the rows in order, each a CI Run with its `project`. Add `running=true` for only the CI Runs that count as running towards a Ticket's CI result: queued or in progress, each at its latest attempt, tracked against a Ticket rather than a CI Rule, on that Ticket's latest pushed commit (or, under Merge + Push, any it pushed since it was last sent back) and not of an Ignored workflow, newest started first. A `running` that isn't true or false is `invalid`.

### CI Rules

A **CI Rule** says "when this workflow fails on this branch, open a Ticket to fix it", for failures no Ticket's own CI catches, such as a red `main` after a hand merge. Add, edit and remove them under **CI Rules**, at the foot of the CI section. Agents have no tool or command to change them. A rule names:

- **Workflow**: one workflow file name, such as `ci.yml`, or `any` for every workflow but the Ignored ones.
- **Branch**: an exact branch name, such as `main`. Globs such as `release/*` are refused.
- **Tickets open as**: **Draft**, for you to look at first, or **Ready**, for the Factory to start fixing straight away.

**Add rule** creates the rule and its own Spec, titled after what it watches, such as `CI: ci.yml on main` or `CI: any workflow on main`. The rule's Tickets will go on that Spec, so its fix Tickets stay together ([ADR 0017](adr/0017-a-ci-rule-opens-its-tickets-on-a-long-lived-spec-of-its-own.md)). Two rules can't watch the same workflow on the same branch. Each rule is listed with its Spec's ID:

```
ci.yml on main: opens Ready Tickets on acme-api-42      Edit  Remove
```

**Edit** changes a rule in place, and its Spec's title changes to match unless you renamed the Spec. **Remove** removes the rule and keeps its Spec as the record of its Tickets. A rule takes effect from when you add it: runs that finished before then never fire it.

When a run of the rule's workflow fails or times out on exactly its branch, the CI Watcher opens a fix Ticket on the rule's Spec, as Draft or Ready as the rule says. The Ticket is an ordinary implementation Ticket. Its title names the workflow and branch, and its description links the run and names the failed jobs:

```text
Fix ci.yml failing on main

**CI** (`ci.yml`) failed on `main` at commit `9f3c2e1`, so the CI Rule on Spec acme-api-42 opened this Ticket to fix it.

- Run: https://github.com/acme/api/actions/runs/7311
- Failed jobs: [test](https://github.com/acme/api/actions/runs/7311/job/2)
- The commit landed acme-api-37, on Spec acme-api-30.
```

The last line is there when the failing commit is where a Ticket landed, such as one you merged by hand. A rule doesn't pile Tickets up: while its Spec has a Ticket that isn't Done or Cancelled, a further failure only adds a comment to that Ticket, linking the new run and naming its failed jobs. Nor does it double up on a Ticket's own CI: a run on a commit the Factory pushed for a Ticket counts towards that Ticket's CI result and never fires a rule. A run fires one rule: when a rule names its workflow and an `any` rule watches the same branch, the rule naming it fires. A run that fires a rule is tracked against it. Only failed and timed_out runs fire a rule: a cancelled run never does.

## Linking GitHub issues

**GitHub Issues** brings a GitHub repository's issues into Code Central, and writes the work's progress back to them. It is off on every Project until you turn it on, and Agents have no tool or command to change it. Turn it on in the **GitHub Issues** section of the same Project Options page, below CI:

- **GitHub Issues**: on or off. Code Central uses your existing `gh` login and stores no GitHub token.
- **GitHub repository**: the repository whose issues come in, as `owner/name`, such as `acme/acme-api`. GitHub Issues can't be on without one. Like CI's, it is filled in from a GitHub Push remote the Factory reported, with a note naming the URL; change it if your issues live in another repository.
- **Filter label**: one GitHub label, such as `code-central`. Only issues carrying it come in. Leave it empty to bring in every open issue.
- **New issues need me**: off by default. When on, each new issue also shows in Needs Me and Focus Mode until you triage it.

For now, the Issue Watcher only checks it can reach the repository: no issues come in yet.

**Save** applies the settings, and every open UI tab shows them. Turning GitHub Issues on is checked first: Code Central checks that `gh` is installed and logged in, can read the repository, that the repository has its issues turned on, and that the account `gh` is logged in as can triage them, which labelling and closing issues needs. If any of these fails, Save refuses, GitHub Issues stays off, and the section says what is wrong and how to fix it, for example:

```text
The account gh is logged in as can't triage the issues of acme/acme-api: Code Central labels and closes them, which needs triage permission or more.
Give the account gh is logged in as triage permission or more on the repository, or log gh in as an account that has it.
```

or:

```text
The repository acme/acme-api has its issues turned off.
Turn Issues on in the repository's Settings on GitHub, under Features, or name the repository whose issues to bring in.
```

The check uses the same `gh`, found the same way, as CI Tracking's: see [Finding `gh`](running-as-a-service.md#finding-gh).

The first time you turn GitHub Issues on, Code Central creates the Project's **Issues Spec**, titled after the repository, such as `GitHub Issues: acme/acme-api`. Each issue arrives on it as an **Incoming Issue**, a Draft Ticket to triage, so the Spec list doesn't grow by one per issue ([ADR 0022](adr/0022-incoming-issues-go-on-one-long-lived-issues-spec-per-project.md)). The section names it:

```
Incoming Issues arrive on acme-api-12.
```

It is an ordinary Spec, with no Stages. Turning GitHub Issues off and on again reuses it, and changing the repository retitles it unless you renamed it. Turning GitHub Issues off keeps the repository, label and New issues need me for the next time you turn it on, and leaves every linked Spec and Ticket as it is.

While GitHub Issues is on, the service runs the Project's **Issue Watcher**, the process that links its issues, whether or not the Factory is running. The GitHub Issues section shows its status below Save. Every 2 minutes it makes sure again that `gh` is installed and logged in, can read the repository, that the repository has its issues turned on, and that the login can triage them. Its status reads as the CI Watcher's does:

- **Issue Watcher: Watching acme/acme-api.**: it can reach the repository's issues.
- **Watching acme/acme-api.** with **GitHub is rate-limiting it until 10:15**: GitHub has refused its calls for its rate limit. It makes no calls until the time GitHub gave, then carries on by itself. When GitHub gives no time, it waits twice as long as it last did, up to 10 minutes. This isn't an error, and sends no Notification.
- **Issue Watcher: Error.**: it can't, with what is wrong and how to fix it, as a refused Save says it. You get one Notification when it goes into an error, not one per check, and once you fix the cause, the next check brings it back to Watching by itself. It reads the **gh path** Option on every check.
- **Error** saying **The Issue Watcher crashed, restarted 3 times**: the Issue Watcher process keeps exiting. The service restarts it by itself, waiting longer after each crash, up to 5 minutes, and shows the last line it wrote; its output is in [the service log](running-as-a-service.md#the-service-log). You get one Notification when it starts crashing, not one per restart.
- **Issue Watcher: Starting; it hasn't reported yet.**: it hasn't checked since the service started or GitHub Issues was turned on. Changing the repository starts it here again.

The service stops the Issue Watcher when you turn GitHub Issues off, when you archive the Project and when the service itself stops.

On each check, the Issue Watcher also brings in the repository's open issues, those with the filter label if you set one, oldest first. Every issue arrives once, including those already open when you turned GitHub Issues on, as a Draft Ticket on the Issues Spec with the issue's title and description. Pull requests never arrive. Each Incoming Issue is linked to its issue, with a badge such as **github acme/acme-api#42**, and its panel has an **Open in GitHub** link to the issue, as every linked Spec's and Ticket's does. An issue never arrives twice: not after the Issue Watcher or the service restarts, not after you turn GitHub Issues off and on again, and not after you unlink its Ticket.

While an Incoming Issue waits for triage, it follows its issue on GitHub on each check. An edit to the issue's title or description changes the Incoming Issue's, recorded in its history as the integration's edit. An issue closed on GitHub shows in Needs Me as **Closed externally**, and reopening it takes it out again. An issue that is deleted shows there too, its panel saying it **was deleted**, and one transferred to another repository or that loses the filter label says it **left the Integration's query**. Its Ticket is never deleted: cancel it, mark it Done or unlink it. Once you change the repository, the Incoming Issues from the old one stay as they are.

### What Code Central writes to an issue

Code Central works out from each Ticket's and Spec's state what its issue should show, and the Issue Watcher makes it so on its next check. Each linked issue carries one **status label**, which the Issue Watcher creates in the repository if it is missing, taking any other `code-central:` label off the issue:

- `code-central:draft` on an Incoming Issue, kept as a Draft or not yet triaged;
- `code-central:requirement` on one you turned To Requirement, while the Requirement's Stages aren't all Done;
- `code-central:building` once its Stages are all Done, while its implementation Tickets are built;
- `code-central:done` once the Requirement is Finished with at least one implementation Ticket Done, and the issue is closed as completed;
- `code-central:wont-fix` on one you chose Won't fix for, or whose Requirement is Finished with nothing landed, which is also closed as not planned.

As a Requirement from an issue goes on, the Issue Watcher posts each step on the issue, in order:

- your context, if you gave To Requirement any;
- the grilling's resolution once the grilling Stage is Done: the latest comment its agent made on the Stage Ticket, never its finish summary;
- the Requirement's text as the spec Stage left it, once that Stage is Done;
- each implementation Ticket as it lands, with its title and the commit that put it into the base, such as `Landed **Fix the login** (acme-api-21) in 4f1c2ab.`. A Ticket Accepted but not yet landed isn't posted, and under Merge + Push with CI Tracking a Ticket is Done only once it is on the remote and green;
- once the Requirement is Finished, what landed, each Ticket with its commit, and the issue is closed as completed. If nothing landed, the issue is closed as not planned with no comment.

Each comment it posts ends with a link back to the Ticket or Requirement in Code Central, and is posted exactly once: not again after the Issue Watcher or the service restarts, even if it stopped between posting it and recording it, and not again after you turn GitHub Issues off and on. Anything that comes due while GitHub Issues is off is written once you turn it back on. Your Rounds and their answers, Failure Reports, Transcripts, Claims and comments on a Ticket are never written to GitHub. Unlinking a Ticket or a Requirement stops every write to its issue.

### Triaging Incoming Issues

An Incoming Issue is a Draft (or Ready) Ticket on the Issues Spec, linked to its issue: its panel's GitHub badge and **Open in GitHub** link open the issue, and its title and description are GitHub's. Its Ticket panel has a **Triage** section with the triage actions, whether or not New issues need me is on:

- **Keep as Draft** leaves it on the Issues Spec as it is, a Draft as it came in, and writes nothing to GitHub. With New issues need me on, it also takes the issue out of Needs Me for good: closing and reopening the issue, editing it, or turning New issues need me off and on never brings it back.
- **To Requirement** opens a box for optional context, then turns the issue into a [Requirement](running-the-factory.md), with its three Stages, whose text is the issue's title as a heading, then its description, then your context under a **Context** heading. The issue's link moves to the Requirement, whose badge reads **github acme/acme-api#7 (outbound)**: from then on Code Central owns its text, so the Stages and you edit it as any Requirement, and edits to the issue on GitHub no longer come in. Code Central only writes to the issue: the Issue Watcher posts your context on it and changes its label to `code-central:requirement`. The Incoming Issue is Cancelled, with the grilling Stage as its Replacement, and keeps its badge, unlinked, and its **Open in GitHub** link, for history.
- **Won't fix** opens a box for an optional reason, then Cancels the Incoming Issue, keeping the reason as its comment, and takes it out of Needs Me. The Issue Watcher then posts the reason on the issue, changes its label to `code-central:wont-fix` and closes it as not planned. With no reason, it only relabels and closes the issue.

With **New issues need me** on, each issue that comes in waits in Needs Me under **New issue**, after Blocked by a Cancelled Ticket and before Closed externally, longest-waiting first. Its row names the issue and when it came in:

```
acme-api-14  Login is broken
github acme/acme-api#7 came in 5m ago
```

Its action panel, in the Inbox and as its Focus Item in Focus Mode, links to the issue and offers the triage actions, so you can triage new issues one at a time. Choosing one moves Focus Mode on to the next item. An issue that comes in while New issues need me is off isn't a New issue, even after you turn it on: triage it from its Ticket panel. An issue closed on GitHub before you triage it shows as **Closed externally** instead, and once you unlink it, it is no longer an Incoming Issue.

## Choosing models for each kind of work

The Factory runs every session with Claude Code's own default model and effort unless something says otherwise. To choose them per Project, open the **Models** section of the Project Options page, below GitHub Issues. It has a row for the **Project default**, then one for each kind of work, the Stage Guide a Ticket runs under:

| Kind of work | Tickets |
| --- | --- |
| `grilling`, `spec`, `tickets` | A Requirement's Stages |
| `map-chart`, `map-grilling`, `map-research`, `map-task` | A Map's Tickets |
| `prototype` | Prototypes |
| `implementation` | Every other Ticket |

Each row has a **Model**, typed in as `claude --model` takes it, such as `opus` or `claude-opus-5-5`, and an **Effort**, one of the valid levels: `low`, `medium`, `high`, `xhigh` and `max` until the Factory reports the ones its `claude` accepts, and then those. Once the Factory has [reported what its `claude` accepts](running-the-factory.md#what-claude-accepts), the Model also suggests the model aliases it reported, such as `fable`, `opus` and `sonnet`, though you can still type any other. It suggests too each model ID the Factory's sessions have run with, such as `claude-opus-5-5`, as their usage arrives. Both start at **Default**: an empty Model, and the first Effort choice. Default says what it inherits: a kind's row inherits the Project default, as in `Default (opus)`, and the Project default shows the flag the Factory reported it was started with, as in `Default (Factory flag: sonnet)`, or else `Default (Claude Code's default)`. For example, to run Opus everywhere but implement at `xhigh` effort, set the Project default's Model to `opus` and the implementation row's Effort to `xhigh`.

**Save** replaces every row at once, and every open UI tab shows them. A model that is blank or spans more than one line, or an effort outside the valid levels, is refused, with the reason under the table. A Ticket's own `model` and `effort` Tags, which its Ticket panel sets, override its row; [Model and effort](running-the-factory.md#model-and-effort) gives the whole order. A change applies from each Ticket's next turn, so there is no need to restart the Factory.

## Stage Guides

Code Central gives each Claim a Stage Guide, its instructions for that kind of work. A guide is the Project's own, and only a named Project setting can change what it says. There are two so far, each a checkbox in the **Stage Guides** section of the Project Options page, below Models, with its own **Save**; every open UI tab shows a change, and an Archived Project shows them read-only.

- **Subagent Exploration** is on by default. While it is on, the implementation guide tells the agent to ask an Explore subagent when understanding existing code means reading across several files, and to keep its answer, not the files. Turn it off for a Project whose agents have no subagents or can't afford them: the guide then has no such line and no empty bullet where it was.
- **Model Assignment** is off by default. While it is on, the tickets guide tells the agent to recommend an `effort` for each Ticket it creates, and a `model` too, one of the aliases the Factory [reported its `claude` accepts](running-the-factory.md#what-claude-accepts) (the line names only the effort until it has reported), and to pass them in `tags` to `create_ticket`. It also tells the agent to give the model, the effort and a one-line reason on each line of the finish summary's numbered breakdown, and to update the Tags with `set_tags` on a redo. The Tags then decide each Ticket's [model and effort](running-the-factory.md#model-and-effort), over the Models section's rows. Turn it on when you want the agent that breaks a Spec into Tickets to size each one's model and effort, instead of every Ticket running on its kind of work's row.

A Project stored before a setting existed reads it as its default, so nothing changes until you change it. Every Project read shows them as `stage_guides`, as in `{"subagent_exploration": true, "model_assignment": false}`. Only you set them, from Project Options or over HTTP, and agents have no tool or command for them:

```sh
curl -X POST localhost:7733/api/p/acme-api/set_subagent_exploration -d '{"on": false}'
curl -X POST localhost:7733/api/p/acme-api/set_model_assignment -d '{"on": true}'
```

A change applies from the next Claim and the next `get_stage_guide` call. A session already running, or one resumed after a Round, keeps the guide in the system prompt it started with, unlike Models, which apply from each Ticket's next turn. To read the guide your Project's agents get, `GET /api/p/{key}/stage-guides/{stage}` or `code-central get-stage-guide <stage>`, which prints the configured Project's guide (`--project` names another).

Another optional line follows the same pattern: one named setting, whose zero value may keep the line in or leave it out, and one fragment in `core/stageguides/fragments/`, which a placeholder line in the guide (`<!-- subagent-exploration -->`, `<!-- model-assignment -->`) marks the place of. [ADR 0034](adr/0034-a-stage-guide-can-vary-by-project-only-through-named-project-settings-that-the-human-sets.md) records the decision.

## Auto-accept

Auto-accept lets finished work you would accept anyway skip In Review: when a Ticket of a kind you have switched on finishes, `finish` moves it to In Review and, in the same write, carries out your accept, so it never shows in Needs Me. A Ticket's **Auto-accept kind** is worked out from what it carries, and each kind is one switch, all off by default:

| Kind | Key | What it covers |
|---|---|---|
| Grilling outcomes | `grilling` | A finished grilling Stage, which goes on to its spec Stage |
| Spec recommendation | `spec` | A finished spec Stage, accepted by the choice its agent recommended: Implement or Split into Tickets |
| Tickets Stage | `tickets` | A finished tickets Stage, so the Tickets it wrote are built |
| Implementation results | `implementation` | Every other finished Ticket: feature work, a Small Change, CI fix, CI Rule and Incoming Issue Tickets, and `Implement:` Tickets |
| Merge results | `merge` | A Spec merge Ticket only, the one Code Central opens when a Spec Branch conflicts with the base |

Prototypes and Map Tickets have no kind and are never auto-accepted. An auto-accepted Ticket goes where your own accept would take it, Accepted when its finish named a branch and Done otherwise, and the Move is recorded as yours with its cause, so the Factory lands it, CI Tracking gates it and Land per Spec gathers it unchanged. An auto-accepted Implement on a Requirement whose tickets Stage is still Draft creates its implementation Ticket as Draft. A spec Stage already In Review from before recommendations existed stays for you to accept.

In the UI it is the **Auto-accept** section of the Project Options page, below Stage Guides: one checkbox per kind, a line saying Prototypes and Map Tickets are never auto-accepted, and one **Save**. An auto-accepted Move shows in the Ticket's **History** as `In Review → Accepted by human, Auto-accepted (Project setting: Implementation results)`, naming the kind; the Ticket and Spec rows get no badge, and nothing appears in Needs Me.

Every Project read shows the setting as `auto_accept`, as in `{"grilling": false, "spec": false, "tickets": false, "implementation": true, "merge": false}`. Only you set it, over HTTP; agents have no tool or command for it, and a kind you leave out of the map keeps its value:

```sh
curl -X POST localhost:7733/api/p/acme-api/set_auto_accept -H 'X-Code-Central-Actor: human' -d '{"kinds": {"implementation": true, "merge": true}}'
```

Turning a kind on affects only Tickets that finish afterwards: nothing already In Review is accepted. [ADR 0037](adr/0037-auto-accept-happens-within-finish-on-the-humans-behalf-and-only-the-human-can-turn-it-on.md) records the decision.

### The `auto-accept` Label

To auto-accept one Ticket, or everything on one Spec, whatever the Project's switches say, give it the reserved `auto-accept` Label. On a Ticket or its Spec it turns every kind on for that Ticket, so a Requirement labelled `auto-accept` runs from grilling to its last implementation Ticket without you, and the Label on a Long-lived Spec such as Small Changes auto-accepts every Small Change. Prototypes and Map Tickets are still never auto-accepted. The Move's cause tells you which switch let a Ticket through: `ticket-label`, `spec-label` or `project`, in that order when more than one applies.

Adding the Label also accepts, in the same write, every Ticket it now covers that is already In Review, except a spec Stage with no recommendation. Removing it changes nothing already accepted; only later finishes go back to In Review.

The Label is yours alone. `add_labels`, `remove_labels`, `create_ticket` and an import refuse it as `invalid` from an agent or an Integration, so MCP and the CLI can't name it. Add or remove it over HTTP as the human:

```sh
curl -X POST localhost:7733/api/p/acme-api/add_labels -H 'X-Code-Central-Actor: human' -d '{"target": "acme-api-12", "labels": ["auto-accept"]}'
```

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
