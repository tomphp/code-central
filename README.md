# Code Central

Code Central is one service that holds a Project's Specs and Tickets. Agents work them through a fixed workflow: a Requirement goes through the grilling, spec and tickets Stages, then becomes implementation Tickets, and the human answers Rounds and accepts the work in the UI along the way. You run it on your own machine, and every repository you set up as a Project talks to it.

## The guides

Read them in this order:

1. [Setting up a Project](docs/setting-up-a-project.md): what to install first, then setting a brand new or an existing repository up as a Project, and the cases where `init` behaves differently: a repository you don't own, a second clone or worktree, re-running it after an upgrade, and agents in a sandbox or container.

## Running the service

[Running Code Central as a service](docs/running-as-a-service.md) covers installing, upgrading and downgrading the service, and its versions.
