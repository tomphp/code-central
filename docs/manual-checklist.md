# Manual checklist for v1

Run this by hand once v1 is built (`make build`). These paths can't be automated in the test suites. Record the date, the version (`code-central --version`) and the result of each step.

## `service install`

- [ ] macOS: `code-central service install`; `launchctl print gui/$(id -u)/com.github.tomoram.code-central` shows it running; `curl -sI http://localhost:7733/api/workflow` shows `X-Code-Central-Version`.
- [ ] Log out and back in: the service is running.
- [ ] `kill -9` the service: launchd starts it again.
- [ ] Install through a symlink on PATH: the plist names the symlink, not its target. Replace the target with a new build and run `code-central service restart`: the new version answers.
- [ ] Run `service install` again: the plist is rewritten and the service restarts.
- [ ] Linux: the same steps with `systemctl --user status code-central`, including a re-login and a `kill -9`.
- [ ] Set `port: 7744` in `~/.config/code-central/settings.yaml`, restart, and reach it with `CODE_CENTRAL_URL=http://localhost:7744`. Remove it again.
- [ ] Occupy 7733 with another program: the service refuses to start and names the address.
- [ ] A CLI built from another tag warns once on stderr and still works.

## Upgrading and downgrading

- [ ] Once a release bumps the store `format`: upgrade a real store (`make build`, copy onto `PATH`, `code-central service restart`). The log says `migrated the store from format <n> to <m>`, `code-central.format-<n>.bak` sits next to the store, and agents keep working through the restart.
- [ ] Downgrade by the steps in `docs/running-as-a-service.md`: the old version starts on the `.bak` store, and the old binary refuses the upgraded store with `unknown format`.

## Docker Desktop for Mac

- [ ] From a container, `curl http://host.docker.internal:7733/api/workflow` reaches a service bound only to loopback (unverified assumption in the design).
- [ ] Claude Code inside the container, with `CODE_CENTRAL_URL=http://host.docker.internal:7733`, lists the `code-central` MCP tools.

## Linux Docker with `--bind`

- [ ] `code-central service install --bind 172.17.0.1` (the bridge IP from `ip addr show docker0`); serve prints the LAN warning.
- [ ] A container started with `--add-host=host.docker.internal:host-gateway` reaches `http://host.docker.internal:7733/api/workflow`.
- [ ] The service is not reachable from another machine on the LAN.

## `sbx` (Docker Sandboxes)

- [ ] After `sbx policy allow network localhost:7733`, a sandboxed agent reaches the MCP endpoint (check whether the proxy rewrites `Host`; the allowlist covers both).
- [ ] An older `code-central` copy inside the sandbox warns about the version but keeps working.

## Real Claude Code

- [ ] `code-central init` in a repository; a new session lists the `code-central` MCP server.
- [ ] The PreToolUse hook `code-central hook session-id` adds `session_id` and `client_version` to `claim`, `set_session` and `post_round` (and `working_dir` to `claim` and `set_session`) through `updatedInput`, and the normal permission prompt still appears.
- [ ] `CLAUDE_CODE_SESSION_ID` is set in the session's Bash tool and matches the hook's `session_id`.
- [ ] The PostToolUse hook `code-central ping` keeps a Claim from turning Silent, and never slows or blocks a tool call, even with the service stopped.
- [ ] `code-central service restart` while a session is mid-`wait`: `wait` resumes, and the MCP server reconnects without the session marking it failed.
