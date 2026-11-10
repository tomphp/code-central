# The service starts and supervises a CI Watcher per Project

CI Tracking needs something to ask GitHub about each pushed Ticket's CI Runs, and it has to keep doing that whether or not the Factory runs. We decided the service starts one **CI Watcher** per Project with CI Tracking on: a separate process, run as a hidden subcommand of the same binary (`code-central ci-watcher --project <key>`) through the service's own executable path. It starts with the service and when CI Tracking is turned on, and stops when CI Tracking is turned off, when the Project is archived, and when the service stops.

The CI Watcher talks to Code Central over the HTTP API only, as an ordinary agent client (ADR 0004): it reads `get_ci_watch` and reports what it sees. The service never calls GitHub itself (ADR 0002), so even the check before CI Tracking turns on runs the CI Watcher once, in its check mode.

Its stdin is a pipe from the service, and it exits when that pipe closes. That is how the service stops it, and it is also what makes sure none is left behind: a stopped Windows scheduled task (ADR 0011) ends the service but not its children, and a service killed outright on any OS can't stop them either, but either way the pipe closes when the service's process ends.

A CI Watcher that exits without being stopped has crashed. The service restarts it after a backoff: 1 second, doubling after each crash to at most 5 minutes, and back to 1 second once one has run for 10 minutes. While it crashes, the service, not the CI Watcher, reports its status: an error saying "crashed, restarted N times" with the last line it wrote to stderr. That status holds until the CI Watcher has run 10 minutes from its restart, and the CI Watcher's own reports are held back until then, so one that reports Watching before crashing again neither hides the crashes nor sends a Notification for each: like any error, going into it sends one. What the CI Watcher writes, and each start, stop and crash, goes to the service's log.

## Considered Options

- **In the Factory.** Rejected: the human wanted CI Tracking independent of the Factory, so CI keeps being watched, and a failure still reaches Needs Me, when no Factory is running.
- **In the service process.** Rejected: ADR 0002 and ADR 0004 keep external calls out of the service, which only stores and serves. Running `gh` and waiting on GitHub there would put a slow, failing outside dependency inside the one process every client depends on.
- **A separately installed binary.** Rejected: it could fall out of step with the service's version, and would be one more thing to install and put on the service's `PATH`. Running the service's own executable keeps them the same version with nothing extra to install.

## Consequences

The CI Watcher's status lives in the service's memory: a restarted service has none until its CI Watchers report again, and never shows a stale Watching. The service reads which Projects should have one on each Project change, and every minute in case it missed one. A CI Watcher that ignores its closed stdin is killed after 10 seconds when the service stops it; that wait holds up no other Project's CI Watcher, and a Project turned back on meanwhile gets its new one once the old one has exited. Supervision is tested on every OS in CI, Windows included, through the real binary: the CI Watcher is gone after the service is stopped or killed.

The supervision is not the CI Watcher's own: the service runs every kind of Watcher this way, by kind per Project, and keeps each one's status by kind. The Issue Watcher, which runs the GitHub Issues Integration, is the second kind ([ADR 0020](0020-an-integration-runs-as-a-watcher-supervised-by-the-service.md)).
