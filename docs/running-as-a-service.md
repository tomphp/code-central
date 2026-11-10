# Running Code Central as a service

Agents can only reach Code Central while `code-central serve` is running. The usual setup is a login item.

## Install

Download the archive for your OS and CPU from [GitHub Releases](https://github.com/tomphp/code-central/releases), with the release's `checksums.txt`. Each archive holds both binaries, `code-central` and `code-central-factory` (`.exe` on Windows), and is named `code-central_<tag>_<os>_<arch>`: `.tar.gz` for `linux` and `darwin`, `.zip` for `windows`, on `amd64` or `arm64`. Verify it, unpack it and put both binaries on your PATH:

```sh
sha256sum --ignore-missing -c checksums.txt     # older macOS: shasum -a 256 --ignore-missing -c checksums.txt
tar -xzf code-central_v1.1.0_darwin_arm64.tar.gz
mv code-central code-central-factory ~/bin/   # anywhere on PATH
code-central service install                  # [--store <dir>] [--bind <ip>]
```

On Windows, check the `.zip` with `Get-FileHash` against its line in `checksums.txt`, unpack it with Explorer or `Expand-Archive`, and move both `.exe` files into a folder on your PATH.

The binaries aren't signed:

- **macOS:** Gatekeeper stops a binary that came from a browser download. Clear the quarantine flag once: `xattr -d com.apple.quarantine ~/bin/code-central ~/bin/code-central-factory`. An archive fetched with `curl` has no flag to clear.
- **Windows:** SmartScreen may warn about an unrecognised app the first time you run it. Choose **More info**, then **Run anyway**.

### Building from source

With Go and Node installed, in a clone of the repository:

```sh
make build
cp bin/code-central bin/code-central-factory ~/bin/   # or symlink them; anywhere on PATH
code-central service install
```

`make build` also turns on the repository's `commit-msg` hook in that clone, which refuses commits until cocogitto (`cog`) is on PATH; its message says how to install it.

### What `service install` does

- **macOS:** writes the launchd agent `~/Library/LaunchAgents/com.github.tomoram.code-central.plist` and loads it. Logs go to `~/Library/Logs/code-central.log`.
- **Linux:** writes the systemd user unit `~/.config/systemd/user/code-central.service` (or under `$XDG_CONFIG_HOME`), enables it and starts it. Logs: `journalctl --user -u code-central`.
- **Windows:** writes the scheduled task `~\.local\state\code-central\code-central-task.xml`, registers it for your account as the task `Code Central` (no admin rights needed) and starts it. The task runs `serve --log`, which appends to `~\.local\state\code-central\code-central.log`. See [ADR 0011](adr/0011-service-install-on-windows-is-a-per-user-scheduled-task-not-a-windows-service.md) for why it isn't a Windows Service. It also writes the Start-menu shortcut `Code Central.lnk` (under `%APPDATA%\Microsoft\Windows\Start Menu\Programs`), which makes Notifications show as "Code Central", with their own entry in Settings > Notifications. Without it, as when you run `serve` by hand, they still appear, shown as Windows PowerShell.

The service starts at login and restarts after a crash. The unit records the binary's absolute path as you invoked it, without resolving symlinks, so a symlinked install keeps working when the target is replaced. It also records the store folder, resolved when you install.

`service install` is safe to run again: it rewrites the unit (and on Windows the shortcut) and restarts the service. Run it again whenever you want to change `--store` or `--bind`.

## Upgrading

Download, verify and unpack the new release's archive as in [Install](#install), replace both binaries on your PATH, then restart the service:

```sh
mv code-central code-central-factory ~/bin/
code-central service restart
```

Building from source instead: `make build && cp bin/code-central bin/code-central-factory ~/bin/`, then `code-central service restart`. The release notes on each GitHub Release list its features, fixes and breaking changes.

`service restart` restarts the installed service. The service never watches for its binary being replaced, so the restart is what picks up the new version. A restart loses nothing: Claims, Rounds and all other state live in the store, the service finishes in-flight writes before it stops (on Windows `/End` terminates it instead, which loses nothing either, because every write is flushed before it is acknowledged), and `code-central wait` retries until the service is back. Keep restarts short, because Claude Code marks an MCP server failed after about 31 seconds.

Until you restart, a newer hook or CLI may send a field the old service doesn't know, and the call fails with `invalid JSON: json: unknown field "<name>"`. Since the service learned to name the versions (see [Versions](#versions)), that error names both and says to run `code-central service restart`. A service from before that change doesn't know `client_version` itself, so a new hook's first `claim`, `set_session` or `post_round` fails once on `unknown field "client_version"` with no hint. Restarting the service fixes it.

### The store is migrated at startup

`store.yaml` records the store's `format`. When a new binary finds an older one, `serve` migrates the store before it binds the port. There is no separate `migrate` command, so a login item never crash-loops after an upgrade. Startup runs in this order:

1. Roll forward any Import that was interrupted.
2. Migrate the format: every file is run through the chained steps (1→2→3…) into a staging folder next to the store (`.code-central.migrating`). Only once the whole staged store is on disk is the old store renamed to `code-central.format-<n>.bak`, where `<n>` is the format it had, and the staging folder renamed into place. Serve prints `migrated the store from format <n> to <m>` with the backup's path on stderr (the service log).
3. Load and strictly validate every file.
4. Bind the port.

If a step fails, the original store is untouched, the staging folder is deleted, and the service refuses to start, naming the step and the file. A store with a newer `format` than the binary knows is refused by name (`unknown format`), so an older binary never misreads it. A 10,000-Ticket store migrates and loads in well under the 10 seconds the end-to-end suite allows on Linux and macOS. On Windows it takes about 25 seconds, so CI allows a minute there for now.

On Windows, a migration doesn't wait for each staged file to reach the disk, because flushing 10,000 files one by one takes minutes. If the power fails or Windows crashes within a few seconds of a migration, some of the migrated files may not have been written, and the service then refuses to start, naming a file that fails to load. Your data is still in `code-central.format-<n>.bak`: restore it as in [Downgrading](#downgrading), but keep the new binary, so the next start migrates again. Only the migration takes this risk; every other write reaches the disk before the service acknowledges it.

The service keeps one backup per format it has migrated from and never deletes them. If a backup for that format already exists (say you downgraded and upgraded again), the migration stops and names it: move it aside and restart. Delete old backups yourself once you're happy with the new version.

### Downgrading

Downgrading is manual and loses every write made since the upgrade:

```sh
launchctl bootout gui/$(id -u)/com.github.tomoram.code-central   # macOS; Linux: systemctl --user stop code-central; Windows: schtasks /End /TN "Code Central"
cd ~/.local/share                      # or wherever your store lives (CODE_CENTRAL_STORE, --store)
mv code-central code-central.upgraded  # keep it until you're sure
mv code-central.format-<n>.bak code-central
cp <old release>/code-central ~/bin/   # the binary that understands format <n>
code-central service install           # rewrites the unit and starts the old version
```

The old binary refuses the upgraded store (`unknown format`), so you can't run it against the wrong one by mistake.

### When to run `init` again

Files written by an older `init` keep working: the `.mcp.json` URL, the port, the MCP server name and the hook commands (`code-central hook session-id`, `code-central ping`) never change within v1, and the generated `docs/agents/` files name operations only, which v1 changes additively. Nothing detects stale files, so run `code-central init` again in a repository when:

- the release notes mention new or changed teaching files, hooks or `docs/agents/` content, or
- you want the generated files' header comment to record the current version.

Running it again rewrites the files it generated without asking, and asks before replacing any file you've changed.

## The service log

The service writes its log to stderr, or appends it to the file `serve --log` names; [What `service install` does](#what-service-install-does) says where each OS keeps it. Besides startup and migration messages, it holds one line for each MCP tool call an agent makes:

```text
code-central serve: mcp get_ticket project=acme id=acme-2 bytes=1534
code-central serve: mcp create_spec project=acme id=acme-4 bytes=96
code-central serve: mcp find_tickets project=acme bytes=2210
code-central serve: mcp get_ticket project=acme id=acme-99 bytes=71 error=not_found
```

- `mcp <operation>` is the tool called, and `project` the Project its URL names.
- `id` is the Ticket or Spec the call names (a bare number is shown as `<key>-<n>`), or else the one it returns, such as the Spec `create_spec` made or the Ticket `claim` gave. It is left out when there is neither.
- `bytes` is the length of the text the agent is given, so the context the call loads into the agent.
- `error=<code>` ends the line of a call the service rejected; `bytes` is then the error's size.

Calls to the HTTP API, from the CLI and the UI, are not logged. [Usage](#usage) adds these calls up per operation in the UI. To add up what one session's Code Central calls load, filter the log by time or `project`; the [Context Report](context-report.md) measures everything else an agent loads, from Claude Code's session logs.

## Versions

A release's binary is stamped with its tag, such as `v1.1.0`, and `make build` stamps `git describe --tags --always --dirty`, such as `v1.1.0-3-g0cf9d18-dirty`. `code-central --version` prints it, and the service sends it in the `X-Code-Central-Version` response header.

A client sends its own version in the `X-Code-Central-Client-Version` request header. The CLI sends it on every request, and any HTTP client may. MCP calls can't carry a header from the CLI, so an MCP client sends it as a top-level `client_version` field instead, which every operation accepts without listing it and which wins over the header. When the service rejects a request for an unknown field, the error says whether the versions are the likely cause:

- the client's version differs from the service's: ``invalid JSON: json: unknown field "working_dir": the caller is version v1.9 and the service is v1.8; run `code-central service restart` ``
- the client sent no version: ``invalid JSON: json: unknown field "x" (the service is v1.8; if your client is newer, run `code-central service restart`) ``
- the versions are equal: the field is just unknown, and the message says no more.

Versions are compared for equality only. Other errors (a wrong type, trailing data) never get the hint.

A CLI of a different version keeps working but warns on stderr. When both versions are clean tags, the warning names the older side: "run `code-central service restart`" when the service is older (you replaced the binary but didn't restart), or "this CLI is older than the service" (for example a stale copy inside a sandbox).

## Changing the port

The port is 7733, and every repository's `.mcp.json` says `${CODE_CENTRAL_URL:-http://localhost:7733}`. If something else really needs 7733, the service refuses to start and names the conflict. To move Code Central without touching any committed file:

1. Set the port in the local settings file, `~/.config/code-central/settings.yaml` (or `$XDG_CONFIG_HOME/code-central/settings.yaml`):

   ```yaml
   port: 7744
   ```

2. Point agents and the CLI at it, for example in your shell profile: `export CODE_CENTRAL_URL=http://localhost:7744`. Claude Code expands `${CODE_CENTRAL_URL}` in `.mcp.json`.
3. `code-central service restart`.

Unknown keys or a bad value in `settings.yaml` stop the service and name the file.

## Options

Preferences you change while Code Central runs are **Options**, not settings. Set them in the UI: click **Options** in the **Admin** section of the Inbox's left pane. Each choice saves the moment you make it, every open browser follows it at once, and **Back to the Inbox** returns you to where you were.

There are two Options so far: *Focus Mode, when nothing needs you*, and **gh path**, which [Finding `gh`](#finding-gh) describes.

*Focus Mode, when nothing needs you* is **Activity Dashboard** (the default) or **All clear**.

Focus Mode shows the dashboard whenever nothing needs you, unless you choose **All clear**: a calm All clear message and nothing else. Until the Options are read, or when they can't be, it shows All clear. The dashboard has a blue block with the number of Ready Tickets and an amber block with the number of In Progress Tickets, and below them a list of the In Progress Tickets. Each entry shows the Ticket's ID and title, its Project (unless Focus Mode is filtered to one), the Requirement and Stage for a Stage Ticket, and its Session's latest event, such as `Read: core/core.go · 2m ago`. Click an entry to open its Ticket in the Inbox. When a Project in scope that isn't Archived has CI Tracking on, a third, teal block, **CI Running**, counts the CI Runs still going (queued or in progress) that count towards a Ticket's CI result: each at its latest attempt, on its Ticket's latest pushed commit, and not of an Ignored workflow. It leaves out what the CI view still shows, CI Rules' runs, ignored runs and runs on a Ticket's earlier pushes, so its number can be lower than the CI view's runs still going. With nothing running it reads 0. Click it to open the CI view on Focus Mode's Project, or All Projects, with **Back to Focus Mode**. Below the In Progress list, a **CI Running** list shows those runs, newest started first, or "No CI Runs running" when there are none. Each entry shows the workflow's name, the Ticket's ID, its Project (unless Focus Mode is filtered to one), the branch, the short commit, and whether it is queued or running and for how long, such as `acme-4 · abc1234 · running 3m 0s`. Click an entry to open its Ticket in the Inbox, as an In Progress entry does, or its **View on GitHub** link to open the run on GitHub. If the CI Runs can't be read, the block stays and the error shows in the list's place until the next change reads them again. Without CI Tracking in scope there is no third block. The counts cover only the Project Focus Mode is filtered to, and leave out Archived Projects. They include blocked Ready Tickets and Stage Tickets, so they can be higher than the Inbox's State counts, which leave out the Stage Tickets folded under each State view. The dashboard follows the live feed. A block pulses when its number changes, and an entry pulses each time its Ticket changes, a new Session event included, at most about every 5 seconds for a busy Session; an entry that joins the list pulses once. A CI Running entry pulses when its run's status or attempt changes, a re-run's next attempt staying the same entry. With your system set to reduce motion, the pulse is a brief brightness flash instead. The dashboard gives way to a Focus Item as soon as something needs you. The "You skipped N" screen is unchanged.

Scripts can read and set Options over the HTTP API, sending `X-Code-Central-Actor: human` to change them: `GET /api/options`, and `POST /api/set_options` with a body such as `{"focus_clear": "all_clear"}` or `{"gh_path": "/opt/tools/gh"}`.

The service keeps Options in its store, in `options.yaml` beside `store.yaml`, so they survive a restart and a cleared browser. There is no file until you first change one, and no MCP tool or CLI command changes them. `settings.yaml` is still the place for what the service needs at startup, such as the port.

## Finding `gh`

CI Tracking (see [Watching CI](setting-up-a-project.md#watching-ci)) reaches GitHub through `gh`, the GitHub CLI, and its own login: Code Central stores no GitHub token. The service itself never calls GitHub. It runs its own executable as the CI Watcher, a hidden `code-central ci-watcher` subcommand, which finds and runs `gh`. Before CI Tracking turns on, the service runs the CI Watcher once in a check mode. The check finds `gh`, runs `gh auth status`, and reads the repository. It runs again when you change the repository while CI Tracking is on. Turning CI Tracking off runs no check.

The CI Watcher finds `gh`:

1. at the **gh path** Option, if it is set;
2. else on the service's `PATH`;
3. else in the usual install places: `/opt/homebrew/bin`, `/usr/local/bin`, `/home/linuxbrew/.linuxbrew/bin` and `%ProgramFiles%\GitHub CLI`.

The install places matter because a service gets a shorter `PATH` than your shell. launchd, for example, starts the service with only the system folders on its `PATH`, so a Homebrew `gh` is found in its install place instead.

Set **gh path** when `gh` is somewhere else. It is on the Options page, below the Focus Mode Option: type the path to `gh`, or to the folder it is in, and click **Save**. It must be an absolute path, such as `/opt/tools/gh` or `C:\Tools\gh.exe`. Empty the field and save to go back to looking on `PATH` and in the install places. When gh path is set, the CI Watcher looks nowhere else.

`gh` runs as the user the service runs as, so log in as that user with `gh auth login`. When the check fails, the CI section of Project Options refuses to turn CI Tracking on. It shows what is wrong, then how to fix it:

| What is wrong | What the CI section says | Its fix |
|---|---|---|
| `gh` isn't found | gh, the GitHub CLI, isn't on the service's PATH or in /opt/homebrew/bin, /usr/local/bin, /home/linuxbrew/.linuxbrew/bin. | Install gh, the GitHub CLI (https://cli.github.com), or set the gh path Option to where it is. |
| gh path names no `gh` | The gh path Option is /opt/tools/gh, but gh isn't there. | Set the gh path Option to where gh is, or clear it to find gh on the service's PATH. |
| `gh` isn't logged in | gh isn't logged in to GitHub: You are not logged into any GitHub hosts. To log in, run: gh auth login | Run `gh auth login` as the user the service runs as. |
| The repository can't be read | gh can't read the repository acme/acme-api: gh: Not Found (HTTP 404) | Check the repository is the owner/name GitHub shows, and that the account gh is logged in as can see it. |

## The CI Watcher

For each Project with CI Tracking on, the service runs a **CI Watcher**: a second `code-central` process, the hidden `code-central ci-watcher --project <key>` subcommand, started through the service's own executable, so it is always the same version and needs nothing else installed. It watches that Project's CI through `gh` and reports what it sees to the service over the HTTP API. You never start it yourself. See [ADR 0018](adr/0018-the-service-starts-and-supervises-a-ci-watcher-per-project.md) for why it is a process of its own.

- **It starts** with the service, for every Project with CI Tracking on, and when you turn CI Tracking on.
- **It stops** when you turn CI Tracking off, when you archive the Project, and when the service stops.
- **It is restarted** if it crashes: after 1 second, then 2, 4 and so on, up to 5 minutes between tries, and back to 1 second once it has run for 10 minutes. While it crashes, the CI section of Project Options says **The CI Watcher crashed, restarted N times**, with the last line it wrote, and you get one Notification.
- **It backs off** when GitHub rate-limits it: it makes no calls until the reset time GitHub gives, or, with none, waits twice as long each time, up to 10 minutes. The CI section shows it still Watching, rate-limited until then, and you get no Notification.
- **It leaves nothing behind.** Its stdin is a pipe from the service, and it exits when that pipe closes. So it ends with the service however the service ends: `code-central service restart`, Ctrl-C, a stopped Windows scheduled task, which doesn't end the service's children itself, or the service being killed.

It writes to [the service log](#the-service-log), beside a line for each start, stop and crash:

```text
code-central serve: started the CI Watcher for acme (pid 41022)
code-central serve: the CI Watcher for acme crashed; restarting it in 1s
code-central serve: stopped the CI Watcher for acme
```

### The Issue Watcher

For each Project with [GitHub Issues](setting-up-a-project.md#linking-github-issues) on, the service runs an **Issue Watcher** the same way: the hidden `code-central issue-watcher --project <key>` subcommand, which reaches GitHub through `gh` and writes to the service over the HTTP API as the `integration` Actor. It starts, stops, restarts, backs off and leaves nothing behind just as the CI Watcher does, with GitHub Issues in place of CI Tracking, and shows its status in the GitHub Issues section of Project Options. It runs whether or not the Factory does. Its lines in the service log name it:

```text
code-central serve: started the Issue Watcher for acme (pid 41023)
code-central serve: stopped the Issue Watcher for acme
```

## Usage

**Usage** shows what agents have consumed: the Code Central calls they made, how full their sessions' context got, and the tokens they used each day, in one Project or in all of them, over the last 7, 30 or 90 days. Open it with **Usage** in the **Admin** section of the Inbox's left pane. It opens on **All Projects** and **Last 7 days**; the **Projects** picker lists every Project, Archived ones marked `(Archived)`, and the **Range** picker offers 7, 30 and 90 days. Both go in the address (`/?view=usage&project=acme&days=30`), so a reload or a bookmark keeps them, and **Back to the Inbox** returns you to where you were. A day is a day in the service's local time, and "the last 7 days" is today and the 6 days before it.

The **Code Central calls** table has one row per MCP operation: its calls, the total and average size of the requests (the arguments' JSON) and of the responses (the text the agent was given, as `bytes=` in [the service log](#the-service-log)), and how many the service rejected. Sizes are in B, KB and MB, with the exact figure on hover. Click a column's header to sort by it, and again to reverse it. Only agents' MCP calls are counted, rejected ones included: not calls to a Project that doesn't exist, and never the HTTP API's, so neither the UI nor the Factory adds to it. Usage loads when you open it and when you change the Project or range; it doesn't follow the live feed.

**Context Size** is how many tokens a session's model read for one response (input, cache write and cache read), which is how full its context window was; a session's peak is the largest of its responses'. The **Context Size by kind** table gives each kind of session (grilling, spec, tickets, implementation, review, subagent, then any other by name) its number of sessions, its average peak and its largest peak. Below it, **Recent sessions** lists the newest 50, newest first: each one's Ticket, which opens that Ticket in the Inbox, its Project, Claim, session ID, kind, model, start, peak Context Size and number of model responses. The model is the raw model ID Claude Code reported, or each one in order of first use, separated by commas, when the session used more than one; it shows `—` when unknown. A session is its session ID plus its subagent, so each review or other subagent is a session of its own, and a session resumed under a later Claim stays one session, shown under its latest Claim. Token counts have thousands separators, with the exact figure on hover. Only sessions whose token usage is posted to the service are counted, so sessions you run by hand aren't; a section with nothing to show says so.

**Token usage** draws one bar per day of the range, oldest first, including days with none, stacked by input, cache write, cache read and output tokens. The **Kind** picker narrows the bars to one kind of session, or shows **All kinds**, without reading Usage again. Point at a bar, or tab to it, to see that day's exact figures and their total. It counts tokens only and never shows a price, since a subscription has none per token.

Click a session's ID in **Recent sessions** to open its Context Size response by response as a line chart at the top of Usage. The chart reads every day Usage keeps, whatever the range, and hovering or focusing a point shows that response's time, Context Size and four token counts. The open session goes in the address too (`/?view=usage&session_project=acme&session=<id>&agent=<id>`, `agent` left out for a main session), so a reload or a bookmark reopens it. **Close session** closes it, and so does the browser's Back when the session was the last thing you opened.

The token usage comes from a Factory, which posts each model response to `POST /api/usage` with `{"session_id": "...", "responses": [...]}`. Each response has its `time`, its `agent` (left out for the main session), its `kind`, its `model` (optional; left out when unknown) and its `input_tokens`, `cache_write_tokens`, `cache_read_tokens` and `output_tokens`. The service files them under the newest Claim with that session ID, which gives the Project, Ticket and Claim, and drops them when there is none. The post counts as a ping, and it always answers `204`, even for an unknown session or a body it can't read.

The service records each call and each response as one line of JSON in `projects/<key>/usage/<YYYY-MM-DD>.jsonl` in its store, one file per Project per local day, which you can read or grep by hand:

```json
{"type":"mcp_call","time":"2026-11-09T14:01:43Z","op":"get_ticket","id":"acme-2","request_bytes":12,"response_bytes":1534}
{"type":"model_response","time":"2026-11-09T14:01:50Z","ticket":"acme-2","claim":1,"session_id":"68888598-50ac-4366-b89d-06e9cdf7a179","kind":"implementation","model":"claude-opus-4-7","input_tokens":4,"cache_write_tokens":1830,"cache_read_tokens":41022,"output_tokens":312}
```

A record that can't be written, or a line that can't be read, is noted in the service log and skipped; it never fails an agent's call or a read. Usage keeps 90 days: the service deletes each file whose day is more than 90 days before today when it starts, and again with the first record of each new day, logging any file it can't delete ([ADR 0016](adr/0016-usage-records-are-append-only-json-lines-files-per-project-per-day-kept-90-days.md)).

Scripts can read it over the HTTP API: `GET /api/usage?project=<key>&days=<7|30|90>`. Leave out `project` for all Projects, Archived ones included; `days` defaults to 7. An unknown Project is `not_found`, and any other `days` is `invalid`. The answer's `operations` are the calls table's rows, `kinds` the Context Size by kind, `sessions` the recent sessions, each with its `models` in order of first use (empty when none is known), and `days` a row for every local day in the range, oldest first, each with its `day` (`YYYY-MM-DD`) and its `kinds`: each kind of session with any usage that day, with its four token counts. `GET /api/usage/session?project=<key>&session=<id>&agent=<id>` reads one session's responses, oldest first, over the 90 days kept: each one's `time`, `context_size` and four token counts. Leave out `agent` for a main session. An unknown Project is `not_found`, and an unknown session has no `responses`.

## Reaching it from Docker: `--bind`

By default the service listens only on `127.0.0.1` and `::1`. Docker Desktop and Docker Sandboxes (`sbx policy allow network localhost:7733`) reach that through `host.docker.internal` or their proxy. Plain Linux Docker can't, so `--bind <ip>` adds one more address, such as the Docker bridge IP:

```sh
code-central service install --bind 172.17.0.1
```

`--bind` adds that address to the listeners and to the `Host` allowlist. It takes one specific IP address, never `0.0.0.0` or `::`.

> **Warning:** never bind a LAN address. Code Central has no authentication, and anything that can reach a bound address can act as the human: answer Questions, accept work and rename Projects. Use only an address that nothing outside this machine can reach, such as a Docker bridge.
