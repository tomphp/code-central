# Manual checklist for v1

Run this by hand once v1 is built (`make build`). These paths can't be automated in the test suites. Record the date, the version (`code-central --version`) and the result of each step.

## The guides

- [ ] Follow every guide listed in `README.md` on a fresh repository, made as the guide makes it, before a release: the commands, prompts and output still match what the guide shows. Fix the guide in the release if they don't.

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

## The UI

- [ ] Open the Options view from the Inbox and from Focus Mode, choose **Activity Dashboard**, and with the Options view open in a second browser, watch it follow the change. Restart the service: the choice is kept, and `options.yaml` sits beside `store.yaml`.
- [ ] With Focus Mode open in a second browser, choose **Activity Dashboard** in the first: Focus Mode switches from All clear to the dashboard without a reload. Its Ready and In Progress counts match the Inbox's State views with their folded Stage Tickets added, and while a real Session runs, its entry's latest event line keeps up. When something needs you, the dashboard gives way to the Focus Item.
- [ ] With the dashboard showing, while a real Session runs, its entry pulses (a short scale-and-glow) as its events arrive, at most about every 5 seconds, and a count block pulses only when its number changes. A Ticket that is claimed while you watch joins the list with one pulse. With the system's reduce-motion setting on, the pulse is a brief brightness flash with no scaling.
- [ ] Open a Map with a few Tickets and Blockers in a narrow and a wide window, in light and dark themes: the graph reads left to right with arrows from each Blocker, every node's type, title and State fit, the progress colours read clearly, the fog node sits dim at the right edge, and the graph scrolls sideways rather than squashing when the pane is narrow. **Decisions so far** and **Out of scope** sit beside the graph, or under it when there's no room.
- [ ] With a Map's Ticket and a Prototype in Needs Me, in light and dark themes: each is its own row with `↳ Map: …` (or the plain Spec's title), and the **Prototype** badge reads clearly beside the title, in the row and on its Focus Item.

## Real Claude Code

- [ ] `code-central init` in a repository; a new session lists the `code-central` MCP server.
- [ ] The PreToolUse hook `code-central hook session-id` adds `session_id` and `client_version` to `claim`, `set_session` and `post_round` (and `working_dir` to `claim` and `set_session`) through `updatedInput`, and the normal permission prompt still appears.
- [ ] `CLAUDE_CODE_SESSION_ID` is set in the session's Bash tool and matches the hook's `session_id`.
- [ ] The PostToolUse hook `code-central ping` keeps a Claim from turning Silent, and never slows or blocks a tool call, even with the service stopped.
- [ ] `code-central service restart` while a session is mid-`wait`: `wait` resumes, and the MCP server reconnects without the session marking it failed.
