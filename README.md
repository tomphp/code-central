# Code Central

Code Central is one service that holds a Project's Specs and Tickets. Agents work them through a fixed workflow: a Requirement goes through the grilling, spec and tickets Stages, then becomes implementation Tickets, and the human answers Rounds and accepts the work in the UI along the way. You run it on your own machine, and every repository you set up as a Project talks to it.

## The guides

Read them in this order:

1. [Setting up a Project](docs/setting-up-a-project.md): what to install first, then setting a brand new or an existing repository up as a Project, and the cases where `init` behaves differently: a repository you don't own, a second clone or worktree, re-running it after an upgrade, and agents in a sandbox or container.
2. [Importing `.scratch/` into a Project](docs/importing-scratch.md): turning the specs and tickets in a repository's `.scratch/` folder into Specs and Tickets in its Project, by asking Claude Code or with `code-central import`.
3. [Working with Claude Code](docs/working-with-claude-code.md): taking one Requirement through every Stage by hand, in a Claude Code session you start, until its implementation lands.

## Running the service

[Running Code Central as a service](docs/running-as-a-service.md) covers installing, upgrading and downgrading the service, and its versions.
