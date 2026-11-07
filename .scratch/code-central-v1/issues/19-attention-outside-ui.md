# How attention reaches the human outside the open UI

Type: grilling
Status: resolved

## Question

Agents now wait on Rounds for hours, so the human only learns about a Question if the UI is open and they happen to look. How does Needs Me reach them beyond that: OS notifications sent by the service, browser notifications from an open tab, a Needs Me count in the tab's title and favicon, or a mix? Which events notify (a new Round, a Failure Report, a Silent Claim, a Ticket reaching `In Review`, "closed externally"), and how is noise avoided when many Tickets change at once? [The attention-first UI](15-attention-ui.md) covers only the open browser, and [Process shape and web stack](10-process-and-stack.md) sends SSE events that only name what changed.

## Answer

Needs Me reaches the human two ways: **OS notifications sent by the service**, which runs as a login item and so works with no browser open, and **a Needs Me count in the tab's title and favicon** (such as `(3) Code Central` plus a badge). Only some reasons notify. The rest only add to the count.

**What notifies.** A Notification is sent when an agent is stopped and waiting on the human:
- a new Round,
- a new Failure Report,
- a Claim becoming Silent, once, when it crosses the 30-minute mark while the service is running.

`In Review`, closed externally, blocked by a Cancelled Ticket, and `Answered` not yet picked up count toward Needs Me but don't notify: no agent is waiting, or the human has just acted.

**Noise.** Events that notify are collected over a 15-second window. A window with one event sends one Notification naming the Ticket and its reason. A window with more sends one summary, such as "3 new Rounds, 1 failure across 2 Projects". There's no per-Ticket limit, because each new Round or Failure Report is a new demand and a Silent Claim already fires only once.

**Clicking.** Where the platform makes it cheap, a click opens the UI: a single-event Notification at `/?view=inbox&ticket=<id>`, and a summary at Needs Me. On Linux this goes through D-Bus or `notify-send --action` (libnotify 0.7.9+), and on macOS through `terminal-notifier` if it is on `PATH`. Otherwise the Notification is text only, for example from `osascript`, and missing `terminal-notifier` loses only the click action.

**No suppression while the UI is open.** v1 sends Notifications even when a tab is visible.

**Turning it off.** A local setting `notifications: on|off` (on by default), next to the port setting, and `serve --no-notify` for a single run. There are no per-reason switches or quiet hours; the OS's Do Not Disturb covers quiet hours.

**Restart.** Nothing happens while the service is down, because it is the only writer. On startup, if Needs Me is not empty, the service sends one summary, such as "5 Tickets need you across 3 Projects". Claims already Silent at startup go into that summary rather than each getting its own Notification.

**Testing.** `notify` is a seam: it subscribes to `events` and needs a timer for the Silent mark, because Silent Claims are computed and produce no change notice. The window, the choice of what to notify and the wording are tested with a fake clock and a fake sender. The real OS sender has one contract test that skips when no notification daemon is present, as in CI, and it isn't part of the three end-to-end tests.

Considered and rejected:
- **Browser notifications:** they work only while a tab is open, when the title count already does, and they add a permission prompt.
- **Anything off the machine** (push, email): breaks one user on one machine and needs credentials.
- **Notifying on every Needs Me reason:** interrupts for work where no agent is waiting.
- **One Notification per event:** floods during bursts such as several Factory loops posting at once.
- **A signed macOS app bundle for click actions:** it belongs with packaging, which is out of scope.
- **Holding back Notifications while a tab is visible:** it relies on a focus signal that can go stale and hide a real alert. Add it later if the noise bothers the human.
