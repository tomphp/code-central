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

- [ ] Open the Options view from the Inbox and from Focus Mode, choose **All clear**, and with the Options view open in a second browser, watch it follow the change. Restart the service: the choice is kept, and `options.yaml` sits beside `store.yaml`.
- [ ] With the installed service and a real `gh`, turn CI Tracking on in Project Options: it saves when `gh` is logged in and can read the repository. After `gh auth logout`, turning it on again (off, then on) is refused with the reason and `gh auth login` as the fix; a repository you can't see is refused naming it. Set the **gh path** Option to a folder without `gh`: the refusal names the Option. Clear it: the Homebrew `gh` is found in its install place even though launchd's `PATH` leaves it out.
- [ ] With the installed service and CI Tracking on, the service log says `started the CI Watcher for <key>` and one `code-central ci-watcher` process runs per such Project. After `code-central service restart` exactly one is running again, and after stopping the service (`launchctl unload`, `systemctl --user stop code-central`, or ending the `Code Central` scheduled task in Task Scheduler) no `ci-watcher` process is left in Activity Monitor, `ps` or Task Manager.
- [ ] With CI Tracking on and Merge + Push to a GitHub repository with an Actions workflow, accept a Ticket: the Factory prints `CI tracks main at …`, and within a minute the Ticket's panel shows a **CI** section with the workflow running, then passed or failed, with its jobs and links that open the run on GitHub. An Ignored workflow's run is greyed and marked ignored.
- [ ] With that Ticket pushed, its row in the Spec view (and in Needs Me or the Activity Dashboard, when it is listed there) shows its CI badge (running, then passed or failed), and the Spec view's sums above its Tickets count it; a Ticket never pushed shows no badge.
- [ ] With CI Tracking on and Merge + Push, let a pushed Ticket's workflow fail: you get one Notification, and the Ticket shows in Needs Me as **CI failed**, ahead of To merge. Its panel and its Focus Item show its CI section with **Land anyway**; choosing it takes it out of Needs Me, and the Factory prints `landing anyway at …` and lands it on its next poll. Under Branch + Push the panel offers no Land anyway.
- [ ] With a pushed Ticket CI failed, under Merge + Push and again under Branch + Push, choose **Re-run failed jobs** on its panel or Focus Item: it leaves Needs Me with its CI result running, the run's failed jobs re-run on GitHub as a new attempt, and the CI view and the Ticket's CI section show that attempt; once it passes, the Ticket's CI result is passed.
- [ ] On a CI failed Ticket, the failure options' note is prefilled with the failed run's link, its failed jobs and the `gh run view` command. **Send back to fix** moves it to Ready with the note; the Factory claims it again in its existing worktree, and its re-accept reports a new pushed commit whose CI result decides. **New fix Ticket** opens a Ready Ticket on the same Spec with the note, and the Factory lands the failed Ticket on its next poll. Both are offered under Branch + Push too.
- [ ] With CI Tracking and **Unblock on push** on under Merge + Push, accept a Ticket that blocks another: once it is pushed, the Factory claims the dependant while its workflow runs, and the Ticket stays Accepted; the dependant's Blockers list shows the Blocker `(Accepted · CI running · no longer blocking)`, and updates without a reload when the CI result changes. Add a second Ready dependant and let the workflow fail: the second becomes blocked and isn't claimed, claiming it by hand names the Blocker `Accepted, CI failed`, the claimed one carries on, and the failed Ticket still lands only once its CI passes or you choose a failure option.
- [ ] With CI Tracking on and a workflow running, open **CI** in the Inbox's Admin section: the CI Watcher shows Watching, the run is listed first with its duration counting up, and when it finishes its row pulses and moves below any still running. **View on GitHub** opens the run, and its Ticket link opens the Ticket.
- [ ] With GitHub Issues on and New issues need me on, a new Incoming Issue shows in Needs Me under **New issue**, its row reading `github owner/repo#n came in …`, and as a Focus Item with its panel; the New issue dot and label read clearly in light and dark themes. **Keep as Draft** takes it out of Needs Me and moves Focus Mode on, and its Ticket panel still offers Keep as Draft in its **Triage** section. On GitHub, the issue gets the `code-central:draft` label, created in the repository if it was missing.
- [ ] On an Incoming Issue, choose **Won't fix…**, type a reason and confirm **Won't fix**: the Ticket is Cancelled with the reason as its comment and leaves Needs Me, and within a check the issue on GitHub gets one comment with the reason and a link back to the Ticket, the `code-central:wont-fix` label in place of `code-central:draft`, and is closed as not planned. Restarting the service posts nothing more.
- [ ] On a fresh store, with Focus Mode open in a second browser, it shows the Activity Dashboard. Choose **All clear** in the first: Focus Mode switches from the dashboard to All clear without a reload, and choosing **Activity Dashboard** again switches it back. The dashboard's Ready and In Progress counts match the Inbox's State views with their folded Stage Tickets added, and while a real Session runs, its entry's latest event line keeps up. When something needs you, the dashboard gives way to the Focus Item.
- [ ] With CI Tracking on in a Project, the Activity Dashboard shows a teal **CI Running** block after Ready and In Progress; with none on, or Focus Mode filtered to a Project with it off, there is no such block. While a pushed Ticket's workflow runs, the block counts its run (but not a run of an Ignored workflow or a CI Rule's), pulses as runs start and finish, and reads 0 once they're done. Clicking it opens the CI view on Focus Mode's Project, and **Back to Focus Mode** returns. Below the In Progress list, the **CI Running** list shows each counted run with its workflow, Ticket ID, branch, short commit and how long it has been running, or "No CI Runs running"; clicking an entry opens its Ticket in the Inbox, and its **View on GitHub** link opens the run on GitHub in a new tab.
- [ ] With the dashboard showing, while a real Session runs, its entry pulses (a short scale-and-glow) as its events arrive, at most about every 5 seconds, and a count block pulses only when its number changes. A Ticket that is claimed while you watch joins the list with one pulse. With the system's reduce-motion setting on, the pulse is a brief brightness flash with no scaling.
- [ ] Open a Map with a few Tickets and Blockers in a narrow and a wide window, in light and dark themes: the graph reads left to right with arrows from each Blocker, every node's type, title and State fit, the progress colours read clearly, the fog node sits dim at the right edge, and the graph scrolls sideways rather than squashing when the pane is narrow. **Decisions so far** and **Out of scope** sit beside the graph, or under it when there's no room.
- [ ] On a Requirement, a Map and a plain Spec (opened from its name in the Ticket panel), in light and dark themes: **New Prototype** sits under the title, its dialog submits, and the new Prototype opens in the Ticket panel.
- [ ] With a Map's Ticket and a Prototype in Needs Me, in light and dark themes: each is its own row with `↳ Map: …` (or the plain Spec's title), and the **Prototype** badge reads clearly beside the title, in the row and on its Focus Item.
- [ ] In Focus Mode, with a few items skipped, submit a Requirement and a Map from **+ New ▾** in the header: the Focus Item stays, the skipped list is kept, a "Requirement added" or "Map added" note shows under the header and goes after about 4 seconds, and with Focus Mode filtered to a Project, the dialog preselects it. The ▾ menu's keyboard behaviour works as in the sidebar (↓ opens on the first item, ↑ and ↓ wrap, Esc returns to ▾, Tab closes), and in light and dark themes the button looks plain beside **Exit**.

## Real Claude Code

- [ ] `code-central init` in a repository; a new session lists the `code-central` MCP server.
- [ ] The PreToolUse hook `code-central hook session-id` adds `session_id` and `client_version` to `claim`, `set_session` and `post_round` (and `working_dir` to `claim` and `set_session`) through `updatedInput`, and the normal permission prompt still appears.
- [ ] `CLAUDE_CODE_SESSION_ID` is set in the session's Bash tool and matches the hook's `session_id`.
- [ ] The PostToolUse hook `code-central ping` keeps a Claim from turning Silent, and never slows or blocks a tool call, even with the service stopped.
- [ ] `code-central service restart` while a session is mid-`wait`: `wait` resumes, and the MCP server reconnects without the session marking it failed.
