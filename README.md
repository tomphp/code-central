# Code Central

Code Central is a tool for driving multiple Claude Code sessions while keeping the user focussed on one question at a time. A Requirement goes through grilling and spec Stages, then either a tickets Stage that splits the Spec into Tickets or **Implement**, which builds it as one Ticket.

## DISCLAIMER

> [!CAUTION]
> This is a vibe coded experiment for personal use.

## Quickstart

### Assumptions

1. You are using a Mac (it might work on other systems but I've not yet tried them)
2. You have Claude Code installed and logged in
3. You want to spend tokens

### Installing

Install the [Matt Pocock Skills](https://www.aihero.dev/skills). Code Central currently only supports this awesome workflow.

```
claude plugins install mattpocock-skills
```

Install Code Central and make sure that both `code-central` and `code-central-factory` are executable and on your PATH.

### Starting

Run `code-central serve` in a terminal window (don't use `install` when just trying it out).

Once started, it is accessible on [http://localhost:7733/](http://localhost:7733/).

### Bootstap a new project

Create a new repository and initialise it.

```
mkdir hello-world
cd hello-world
git init
code-central init
git add .
git commit "initialize code central"
```

At this point, `hello-world` should appear in the UI.

### Developing

Start the factory in your project repository (there are other approaches, but start with this).

```
cd hello-world
code-central-factory
```

> [!IMPORTANT]
> **Windows Users:
> If you get an error about `git` missing from the `PATH`, then run `code-central-factory`
> in Git Bash. You can do this be running the following in your terminal before starting the factory:
>
> `& "C:\Program Files\Git\bin\bash.exe"`

Start adding requirements in the Code Central UI.

Press the **Focus** button, and repond whenever you are prompted.

### Tips

- Start by just adding _Requirements_ (_Maps_ are for large pieces of work and use the `/wayfinder` skill).
- Don't be afraid to add several requirements at a time.
- Don't be afraid to bootstrap multiple projects and work on them concurrently (you will need a factory running per project).
- Try tell a requirement to create a prototype.

### Several repositories

A Project's work can span more than one repository. The one you ran `code-central init` in is its Home Repository, where the Factory runs. Add the others in the **Repositories** section of Project Options, or from the Home Repository:

```
$ code-central repository add web --clone-url git@github.com:acme/web.git
NAME             CLONE URL                         BASE  LOCAL PATH  LANDING          PUSH REMOTE
acme-api (home)  git@github.com:acme/acme-api.git  main  -           Merge (default)  origin
web              git@github.com:acme/web.git       main  -           Merge (default)  origin
```

`--clone-url` is optional: a repository that is never pushed needs only the local path of its clone, and is local-only.

```
$ code-central repository add notes --local-path /home/you/src/notes
NAME             CLONE URL                         BASE  LOCAL PATH           LANDING          PUSH REMOTE
acme-api (home)  git@github.com:acme/acme-api.git  main  -                    Merge (default)  origin
web              git@github.com:acme/web.git       main  -                    Merge (default)  origin
notes            (none: local only)                main  /home/you/src/notes  Merge (default)  origin
```

`code-central repository edit web --clone-url ""` clears a Clone URL, as long as the repository has a local path.

Each repository, the Home included, follows the Project's Default Landing, or has its Own Landing set with `--landing` (`default`, `merge`, `merge+push`, `branch`, `branch+push` or `pull-request`) and its own Push remote with `--push-remote` (empty for the Project's).

The Home Repository need not be pushed anywhere. A common pattern keeps the Home local-only, holding the Project's files and the Factory's workspace, with the pushed repositories beside it. When the Home's clone has no remote of its Push remote's name, the Factory reports none and the Home lists as local-only, while the others push as usual:

```
$ code-central repository list
NAME         CLONE URL                    BASE  LOCAL PATH  LANDING          PUSH REMOTE
acme (home)  (none: local only)           main  -           Merge (default)  origin
web          git@github.com:acme/web.git  main  -           Merge (default)  origin
```

Nothing stops landing for want of a remote: a Ticket lands in the Home once merged, and only the pushed repositories are pushed.

The list is the order accepted work lands in. The Factory clones each repository beside the Home Repository and gives every Ticket a worktree and a branch of the same name in each, named after the Ticket:

```
~/src/acme-api                     the Home Repository
~/src/acme-api.worktrees/acme-4    the Ticket's Home worktree, where its session starts
~/src/web                          cloned by the Factory
~/src/web.worktrees/acme-4         reached with --add-dir
```

[Setting up a Project](docs/setting-up-a-project.md#several-repositories) and [Running the Factory](docs/running-the-factory.md#how-accepted-work-lands) have the rest.


## The guides

Read them in this order:

1. [Setting up a Project](docs/setting-up-a-project.md): what to install first, then setting a brand new or an existing repository up as a Project, adding the other repositories its work spans (`code-central repository add`), and the cases where `init` behaves differently: a repository you don't own, a second clone or worktree, re-running it after an upgrade, and agents in a sandbox or container. It also covers linking a Project to Azure Boards work items, by saved query or WIQL filter with a PAT that has Work Items: Read & write, which come in as Incoming Issues and get comments and a `code-central:` tag back, and, if you fill in **Work Item States**, their State moved once as the work first reaches In Progress, Accepted or Done (**Hide Code Central** keeps the comments but writes no tag, no link back and no Spec or Ticket IDs); and linking it to its repository's GitHub issues, which come in as Incoming Issues to triage: with **New issues need me** on, each new one waits in Needs Me and Focus Mode as **New issue** until you triage it; **Keep as Draft** parks it as a Draft, out of Needs Me for good, writing nothing to GitHub; **To Requirement** makes a Requirement of it, with your optional context, and from then on only writes to the issue; and **Won't fix** Cancels it and closes the issue as not planned, posting your optional reason there as a comment. Code Central labels each linked issue with where its work stands, from `code-central:draft` through `code-central:requirement` and `code-central:building` to `code-central:done`, posts each step of a Requirement's progress on its issue (the grilling's resolution, the Requirement's text and each Ticket as it lands, with its commit) exactly once, and closes the issue when the Requirement is Finished: as completed with what landed, or as not planned if nothing did.
2. [Importing `.scratch/` into a Project](docs/importing-scratch.md): turning the specs and tickets in a repository's `.scratch/` folder into Specs and Tickets in its Project, by asking Claude Code or with `code-central import`.
3. [Working with Claude Code](docs/working-with-claude-code.md): taking one Requirement through every Stage by hand, in a Claude Code session you start, until its implementation lands.
4. [Running the Factory](docs/running-the-factory.md): taking the same Requirement through every Stage with `code-central-factory`, which claims the work and lands it by the Project's Landing Method, leaving you to answer Rounds and accept the work (and, under the Branch Landing Method, to merge it; under the Pull Request Landing Method, to merge the pull request that a Pull Request Watcher opens on GitHub or Azure Repos, after which the Factory fetches the base and lands the Ticket at the host's merge commit). Under Merge with Land per Spec, the accepted Tickets of every Spec but a Long-lived one (a Spec labelled `long-lived`, such as a CI Rule's) gather on a Spec Branch and land together once all are accepted. Under Merge + Push with CI Tracking on, accepted work lands only once its CI on GitHub Actions or Azure Pipelines has passed. While it runs, the Project's dot in the left pane shows its Factory Status. With a Factory Online that supports Chat, a round Chat icon in the lower right corner opens a pop-up for chatting with Claude about the Project. It also runs a Retro, which looks back over the sessions that delivered a finished Ticket or Spec and proposes changes to the agents' environment, read-only, with `/retro`; ask for one with `code-central submit-retro`.
5. [Writing a Factory](docs/writing-a-factory.md): sending Heartbeats from your own Factory so the dot shows it, and answering Chat turns.
6. [Architecture](docs/architecture.md): the modules, what each owns, how they talk to each other (with a diagram), and where the boundaries are weakest.

## Developing the UI

`code-central serve` and `npm --prefix web run dev` run the app with hot reload (Vite serves the UI on :5173 and proxies `/api` to the service). `make serve-components` serves a component browser, a Storybook of the UI's shared components, on http://localhost:6006; `make build-components` builds it into `web/storybook-static` to check it. It needs Node, and is a dev tool only: it isn't in the built UI or in `make test`. Each shared component has a `*.stories.tsx` beside it, whose world the scenario builder makes on the WASM Core, with no service behind it; see [Recipes](docs/agents/recipes.md). `make test`, `make vitest` and both component targets first run `make corewasm`, which builds the real Core to WebAssembly for the UI's tests and stories, into `web/src/api/core/core.wasm` (git-ignored, never in the built UI).

## Running the service

[Running Code Central as a service](docs/running-as-a-service.md) covers installing (download the archive for your OS and CPU from [GitHub Releases](https://github.com/tomphp/code-central/releases), or build from source), upgrading and downgrading the service, its versions, [choosing the store](docs/running-as-a-service.md#choosing-the-store) (files in the data folder by default, one SQLite database file on this machine, never on a network share, or a Postgres database several services may share), [moving to another store](docs/running-as-a-service.md#moving-to-another-store) with `code-central store move --to <kind>`, the Options you set in the UI, and [Usage](docs/running-as-a-service.md#usage), what agents have consumed, with a Storage tab showing where the store is kept and what it holds.

> [!WARNING]
> **On Windows, a power cut or crash just after an upgrade can leave the store unreadable.** When an upgrade changes the store's format, the service migrates it at startup, and on Windows it doesn't wait for every migrated file to reach the disk (that would take minutes on a large store). If Windows goes down within a few seconds of that, the service may refuse to start. Nothing is lost: the old store is kept as `code-central.format-<n>.bak`. See [The store is migrated at startup](docs/running-as-a-service.md#the-store-is-migrated-at-startup) for how to restore it.
