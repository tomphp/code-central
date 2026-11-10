# An Integration runs as a Watcher supervised by the service

[ADR 0004](0004-an-integration-is-a-client-of-the-http-api.md) made an Integration a separate program that talks to Code Central over the HTTP API, running its own polling with its own credentials and configuration. GitHub Issues, the first Integration, asks to be "like the GitHub Actions integration, opt-in per Project", and [ADR 0018](0018-the-service-starts-and-supervises-a-ci-watcher-per-project.md) already proved that pattern for CI Tracking. We decided the GitHub Issues Integration runs as the **Issue Watcher**: a hidden subcommand of the same binary (`code-central issue-watcher --project <key>`), started, stopped and restarted by the service exactly as ADR 0018 describes for the CI Watcher, one per Project with GitHub Issues on. It reaches GitHub through `gh` and its login, as the CI Watcher does, rather than credentials of its own, and the human configures it in the GitHub Issues section of Project Options, not in a file of its own.

This amends ADR 0004's separate program, its own credentials and its own configuration. What ADR 0004 decided about ownership still holds: the Issue Watcher is an HTTP client like any other, it writes as the `integration` Actor, and the service never calls GitHub itself ([ADR 0002](0002-the-service-is-the-only-writer.md)), so even the check before GitHub Issues turns on runs the Issue Watcher once, in its check mode. That check passes when `gh` is found, is logged in, can read the repository, the repository has issues on, and the login has at least triage permission there, which labelling and closing issues need.

## Considered Options

- **A separately installed program, as ADR 0004 describes.** Rejected: it is one more thing for the human to install and keep running, it would need its own credentials and configuration, and it could run a different version from the service. Running the service's own executable keeps them the same version with nothing extra to install, and `gh`'s login is already set up for CI Tracking.

## Consequences

An Integration is supervised like any other Watcher: its status (Watching, or an error with its fix) shows in its section of Project Options, going into an error sends one Notification, and it stops when GitHub Issues is turned off, the Project is archived or the service stops. Turning GitHub Issues off stops the Issue Watcher but leaves every Link as it is, so turning it back on picks up where it left off. A future Integration for another system is one more kind of Watcher.
