# Running Code Central as a service

Agents can only reach Code Central while `code-central serve` is running. The usual setup is a login item.

## Install

```sh
make build
cp bin/code-central ~/bin/            # or symlink it; anywhere on PATH
code-central service install          # [--store <dir>] [--bind <ip>]
```

- **macOS:** writes the launchd agent `~/Library/LaunchAgents/com.github.tomoram.code-central.plist` and loads it. Logs go to `~/Library/Logs/code-central.log`.
- **Linux:** writes the systemd user unit `~/.config/systemd/user/code-central.service` (or under `$XDG_CONFIG_HOME`), enables it and starts it. Logs: `journalctl --user -u code-central`.

The service starts at login and restarts after a crash. The unit records the binary's absolute path as you invoked it, without resolving symlinks, so a symlinked install keeps working when the target is replaced. It also records the store folder, resolved when you install.

`service install` is safe to run again: it rewrites the unit and restarts the service. Run it again whenever you want to change `--store` or `--bind`.

## Upgrading

```sh
make build && cp bin/code-central ~/bin/
code-central service restart
```

`service restart` restarts the installed service. The service never watches for its binary being replaced, so the restart is what picks up the new version. A restart loses nothing: Claims, Rounds and all other state live in the store, the service finishes in-flight writes before it stops, and `code-central wait` retries until the service is back. Keep restarts short, because Claude Code marks an MCP server failed after about 31 seconds.

Until you restart, a newer hook or CLI may send a field the old service doesn't know, and the call fails with `invalid JSON: json: unknown field "<name>"`. Since the service learned to name the versions (see [Versions](#versions)), that error names both and says to run `code-central service restart`. A service from before that change doesn't know `client_version` itself, so a new hook's first `claim`, `set_session` or `post_round` fails once on `unknown field "client_version"` with no hint. Restarting the service fixes it.

### The store is migrated at startup

`store.yaml` records the store's `format`. When a new binary finds an older one, `serve` migrates the store before it binds the port. There is no separate `migrate` command, so a login item never crash-loops after an upgrade. Startup runs in this order:

1. Roll forward any Import that was interrupted.
2. Migrate the format: every file is run through the chained steps (1→2→3…) into a staging folder next to the store (`.code-central.migrating`). Only once the whole staged store is on disk is the old store renamed to `code-central.format-<n>.bak`, where `<n>` is the format it had, and the staging folder renamed into place. Serve prints `migrated the store from format <n> to <m>` with the backup's path on stderr (the service log).
3. Load and strictly validate every file.
4. Bind the port.

If a step fails, the original store is untouched, the staging folder is deleted, and the service refuses to start, naming the step and the file. A store with a newer `format` than the binary knows is refused by name (`unknown format`), so an older binary never misreads it. A 10,000-Ticket store migrates and loads in well under the 10 seconds the end-to-end suite allows.

The service keeps one backup per format it has migrated from and never deletes them. If a backup for that format already exists (say you downgraded and upgraded again), the migration stops and names it: move it aside and restart. Delete old backups yourself once you're happy with the new version.

### Downgrading

Downgrading is manual and loses every write made since the upgrade:

```sh
launchctl bootout gui/$(id -u)/com.github.tomoram.code-central   # macOS; Linux: systemctl --user stop code-central
cd ~/.local/share                      # or wherever your store lives (CODE_CENTRAL_STORE, --store)
mv code-central code-central.upgraded  # keep it until you're sure
mv code-central.format-<n>.bak code-central
cp <old build>/code-central ~/bin/     # the binary that understands format <n>
code-central service install           # rewrites the unit and starts the old version
```

The old binary refuses the upgraded store (`unknown format`), so you can't run it against the wrong one by mistake.

### When to run `init` again

Files written by an older `init` keep working: the `.mcp.json` URL, the port, the MCP server name and the hook commands (`code-central hook session-id`, `code-central ping`) never change within v1, and the generated `docs/agents/` files name operations only, which v1 changes additively. Nothing detects stale files, so run `code-central init` again in a repository when:

- the release notes mention new or changed teaching files, hooks or `docs/agents/` content, or
- you want the generated files' header comment to record the current version.

Running it again rewrites the files it generated without asking, and asks before replacing any file you've changed.

## Versions

`make build` stamps `git describe --tags --always --dirty` as the version. `code-central --version` prints it, and the service sends it in the `X-Code-Central-Version` response header.

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

## Reaching it from Docker: `--bind`

By default the service listens only on `127.0.0.1` and `::1`. Docker Desktop and Docker Sandboxes (`sbx policy allow network localhost:7733`) reach that through `host.docker.internal` or their proxy. Plain Linux Docker can't, so `--bind <ip>` adds one more address, such as the Docker bridge IP:

```sh
code-central service install --bind 172.17.0.1
```

`--bind` adds that address to the listeners and to the `Host` allowlist. It takes one specific IP address, never `0.0.0.0` or `::`.

> **Warning:** never bind a LAN address. Code Central has no authentication, and anything that can reach a bound address can act as the human: answer Questions, accept work and rename Projects. Use only an address that nothing outside this machine can reach, such as a Docker bridge.
