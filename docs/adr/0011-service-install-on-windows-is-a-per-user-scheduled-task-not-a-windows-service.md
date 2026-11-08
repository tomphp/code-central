# service install on Windows is a per-user scheduled task, not a Windows Service

`code-central service install` runs `serve` as a login item that restarts after a crash: a launchd agent on macOS and a systemd user unit on Linux. Porting it to Windows meant choosing what plays that part there. We decided it is a per-user Task Scheduler task, named `Code Central`. Its XML is written to `~\.local\state\code-central\code-central-task.xml`, next to the service's log, and registered with `schtasks /Create /TN "Code Central" /XML <file> /F`. The task:

- runs at the user's logon, as that user, interactively (`InteractiveToken`)
- restarts on failure (`RestartOnFailure`, every minute, up to 999 times)
- has no execution time limit
- starts `code-central.exe serve` with the same serve arguments as launchd and systemd, through `cmd.exe` so its output is appended to `~\.local\state\code-central\code-central.log`, because a task's action can't redirect output itself

Install then starts it with `/End`, which may fail when the task isn't running, then `/Run`. `service restart` runs the same two. Installing again replaces the task, because of `/F`.

The reasons: a task registered for the user's own account needs no admin rights. It runs in the user's session, so its toasts show. And it runs under the user's account with their home folder, so it uses the same store as the CLI and the Factory.

## Considered Options

- **A Windows Service.** Rejected: installing one needs admin rights, and it runs in session 0 under another account, with another home folder. It would find another store, and its toasts would never reach the user's desktop.
- **A Startup-folder shortcut or a `Run` registry entry.** Rejected: both start `serve` at logon but never restart it after a crash, unlike launchd's `KeepAlive` and systemd's `Restart=on-failure`.

## Consequences

`/End` terminates `serve` without a graceful shutdown, unlike launchd and systemd, which interrupt it. This is acceptable, because every store write is atomic and flushed before it is acknowledged, so a terminated `serve` loses nothing it has acknowledged. A restart still looks to agents like any other restart: `code-central wait` retries until the service is back.

`cmd.exe` expands `%NAME%` even inside quotes, so install refuses a binary, store or log path holding `%` or `"` rather than register a task that runs something else.
