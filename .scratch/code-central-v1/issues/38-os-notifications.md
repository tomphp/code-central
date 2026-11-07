# 38: OS Notifications

**What to build:** With no browser open, the human hears about agents stopped and waiting on them: a new Round, a new Failure Report, or a Claim turning Silent. Bursts are gathered into one summary, a click opens the right place where the platform makes it cheap, and the human can turn it all off. See the [spec](../spec.md), section "Needs Me and Notifications".

**Blocked by:** 29, 30, 31

**Status:** resolved

- [x] A `notify` seam subscribes to `events` and runs a timer for the Silent mark
- [x] It notifies only on a new Round, a new Failure Report, or a Claim crossing 30 minutes of silence (once per crossing, while the service runs); `In Review`, `Answered`, blocked and closed externally never notify
- [x] Events are gathered over a 15-second window: one event sends one Notification naming the Ticket and its reason; more send one summary such as "3 new Rounds, 1 failure across 2 Projects"
- [x] On startup, if Needs Me isn't empty, one summary is sent, such as "5 Tickets need you across 3 Projects"; Claims already Silent at startup go into it
- [x] A click opens `/?view=inbox&ticket=<id>` for a single event and Needs Me for a summary, through `terminal-notifier` when it is on `PATH` on macOS, and D-Bus or `notify-send --action` on Linux; otherwise the Notification is text only (for example from `osascript`)
- [x] A local setting `notifications: on|off` (on by default), next to the port setting, and `serve --no-notify` for one run, turn Notifications off
- [x] The window, what notifies and the wording are tested with a fake clock and a fake sender; the real OS sender has one contract test that skips when no notification daemon is present
