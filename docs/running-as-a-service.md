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

`service restart` restarts the installed service. The service migrates the store itself at startup. A restart loses nothing: Claims, Rounds and all other state live in the store, the service finishes in-flight writes before it stops, and `code-central wait` retries until the service is back. Keep restarts short, because Claude Code marks an MCP server failed after about 31 seconds.

## Versions

`make build` stamps `git describe --tags --always --dirty` as the version. `code-central --version` prints it, and the service sends it in the `X-Code-Central-Version` response header.

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
