# Code Central

Code Central is one service that holds a Project's Specs and Tickets. Agents work them through a fixed workflow: a Requirement goes through the grilling, spec and tickets Stages, then becomes implementation Tickets, and the human answers Rounds and accepts the work in the UI along the way. When the destination isn't clear yet, the human starts a Map instead, with **Map** in the **▾** menu beside **+ New**, and agents find the way to it one decision at a time. The same **+ New ▾** sits in Focus Mode's header, beside **Exit**, and submitting there keeps you in Focus Mode where you were, with a brief "Requirement added" or "Map added" note. Once every decision is made, **Write the Spec** turns the Map into a Requirement that skips grilling, since the Map's decisions already did that work. On any Spec, **New Prototype** asks for a cheap, throwaway artifact to try before a decision is made. You run it on your own machine, and every repository you set up as a Project talks to it.

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

Start adding requirements in the Code Central UI.

Press the **Focus** button, and repond whenever you are prompted.

### Tips

- Start by just adding _Requirements_ (_Maps_ are for large pieces of work and use the `/wayfinder` skill).
- Don't be afraid to add several requirements at a time.
- Don't be afraid to bootstrap multiple projects and work on them concurrently (you will need a factory running per project).
- Try tell a requirement to create a prototype.


## The guides

Read them in this order:

1. [Setting up a Project](docs/setting-up-a-project.md): what to install first, then setting a brand new or an existing repository up as a Project, and the cases where `init` behaves differently: a repository you don't own, a second clone or worktree, re-running it after an upgrade, and agents in a sandbox or container. It also covers linking a Project to its repository's GitHub issues, which come in as Incoming Issues to triage: with **New issues need me** on, each new one waits in Needs Me and Focus Mode as **New issue** until you triage it; **Keep as Draft** parks it as a Draft, out of Needs Me for good, writing nothing to GitHub; **To Requirement** makes a Requirement of it, with your optional context, and from then on only writes to the issue; and **Won't fix** Cancels it and closes the issue as not planned, posting your optional reason there as a comment. Code Central labels each linked issue with where its work stands, from `code-central:draft` through `code-central:requirement` and `code-central:building` to `code-central:done`, posts each step of a Requirement's progress on its issue (the grilling's resolution, the Requirement's text and each Ticket as it lands, with its commit) exactly once, and closes the issue when the Requirement is Finished: as completed with what landed, or as not planned if nothing did.
2. [Importing `.scratch/` into a Project](docs/importing-scratch.md): turning the specs and tickets in a repository's `.scratch/` folder into Specs and Tickets in its Project, by asking Claude Code or with `code-central import`.
3. [Working with Claude Code](docs/working-with-claude-code.md): taking one Requirement through every Stage by hand, in a Claude Code session you start, until its implementation lands.
4. [Running the Factory](docs/running-the-factory.md): taking the same Requirement through every Stage with `code-central-factory`, which claims the work and lands it by the Project's Landing Method, leaving you to answer Rounds and accept the work (and, under the Branch Landing Method, to merge it). Under Merge + Push with CI Tracking on, accepted work lands only once its CI on GitHub Actions has passed.

## Running the service

[Running Code Central as a service](docs/running-as-a-service.md) covers installing (download the archive for your OS and CPU from [GitHub Releases](https://github.com/tomphp/code-central/releases), or build from source), upgrading and downgrading the service, its versions, the Options you set in the UI, and [Usage](docs/running-as-a-service.md#usage), what agents have consumed.

> [!WARNING]
> **On Windows, a power cut or crash just after an upgrade can leave the store unreadable.** When an upgrade changes the store's format, the service migrates it at startup, and on Windows it doesn't wait for every migrated file to reach the disk (that would take minutes on a large store). If Windows goes down within a few seconds of that, the service may refuse to start. Nothing is lost: the old store is kept as `code-central.format-<n>.bak`. See [The store is migrated at startup](docs/running-as-a-service.md#the-store-is-migrated-at-startup) for how to restore it.
