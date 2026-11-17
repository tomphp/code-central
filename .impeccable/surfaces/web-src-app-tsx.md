---
version: 1
slug: "web-src-app-tsx"
primary_target: "web/src/App.tsx"
related_targets: ["web/src/styles.css"]
---

# Code Central web UI

Scope: the whole web UI (`web/src`): shell and navigation, Inbox and Needs Me, Focus Mode, Board, Activity, CI, Usage, Options and Project Options, popups and Chat. Mode: Operate.

Audience and job: a solo developer driving several Factories who switches to this tab when its title shows something needs them, answers one thing, and leaves. Must keep every piece of information and every action the current UI shows; layout and navigation may change. Must not read as Linear or Jira, must not be playful or decorative, and must keep State and reason colour coding readable at a glance.

Memorable moment: a Requirement's page draws its route as a line diagram, a station per Stage, lit where the work stands.

Settled in build: Board columns carry their State's line as a stroke under each head; Project ribbons are bars (never rings) from a palette of their own, so they never read as States.

## Direction contract

THESIS: Work moves along fixed routes like a metro network: States are lines, Stages are stations, Needs Me is the interchange where every line meets the human. It refuses the neutral grey tracker with a single blue accent.

OWN-WORLD: Midnight-blue enamel shell rail (#12224a) beside a porcelain work surface (#f4f2ec, white panels); scarlet, cobalt, amber and green line inks as State colours, each State keeping its own ink; station rings (white disc, thick coloured border) as State markers; a humanist grotesk for text, one mono with tabular figures for ids, counts and times. Square-ish 6px corners, hairline rules, no shadows beyond focus.

STORY: The visitor sees what waits on them, why, and for how long, answers it, and trusts that everything else is moving on its line.

FIRST VIEWPORT: Left midnight rail: brand, + New and Focus, Projects with ribbon colour, Views with counts (Needs Me count in amber), States as line keys, Admin. Middle: Needs Me list grouped by reason, each row a station ring, mono id, title with cause, right-aligned waited time. Right: the selected item; a Requirement draws its Stage route across the top, a Ticket shows its State chip and reason (a Ticket's lifecycle is its State, and Small Changes, CI rules and Incoming Issues have no Requirement route), then the Round with Answer as the primary action. Needs Me groups are vertical lines in their reason ink with a station per row.

FORM: Midnight Transit Diagram, a dealt challenger (wayfinding-cartography-signage-midnight-transit-diagram), chosen over my list's position 4 (flight strips); seed:b3dc073f18aa9384.

FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, DESIGN.md, and every shipping raster carrying its provenance
