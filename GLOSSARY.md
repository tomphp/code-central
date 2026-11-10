# Code Central

A local coordination point for AI agent-driven software delivery across many repositories: it holds the work and its state, and shows the human what needs them.

## Language

**Project**:
A single repository whose work is tracked here, however many clones or worktrees of it exist.
_Avoid_: Repo (in domain language), workspace

**Archived Project**:
A Project put away by the human once all its work is finished, which takes no new work until it is brought back.
_Avoid_: Deleted, closed, inactive

**Project Key**:
The short, permanent name that identifies a Project to anything connecting to Code Central.
_Avoid_: Slug, Project ID

**Project Options**:
The page where the human sets a Project's settings, such as its Landing Method and CI Tracking. Unlike an Option, what it sets belongs to one Project.
_Avoid_: Project settings, preferences, config

**Factory**:
A program outside Code Central that claims Tickets and runs agents on them to deliver work.
_Avoid_: Controller, runner, orchestrator

**Spec**:
The description of a piece of work, such as a feature or user story, delivered through one or more Tickets.
_Avoid_: Epic, feature, PRD

**Section**:
A part of a Spec's body that starts at a heading and runs to the next heading at the same level or higher.
_Avoid_: Chapter, part, block

**Requirement**:
A Spec started from the human's free text and defined through Stages before it is built. Its free text is the Requirement's text.
_Avoid_: Prompt, feature request, idea

**Map**:
A Spec that finds the way to a loosely-defined destination through decision Tickets worked one at a time, each marked by a reserved Wayfinder Label. Its body is the index of decisions made, the fog not yet specified and what is out of scope.
_Avoid_: Wayfinder session, roadmap, journey

**Frontier**:
A Map's Draft Tickets that nothing blocks: the decisions that can be started next. Starting one approves it to Ready. It is worked out on each read, never stored.
_Avoid_: Next steps, backlog

**Guided Map**:
A Map whose `guided` Tag is `on`. Whenever it is idle, with every Ticket on it but research Draft, Done or Cancelled, the Factory approves its first frontier Ticket in created order, so nothing else waits on the human or an agent. Turning it off hands starting back to the human's **Start**.
_Avoid_: Autopilot, auto-advance

**Map view**:
The UI's view of a Map: its Destination, **New Prototype**, its Tickets as a left-to-right graph with Blockers as edges and **Start** on each frontier Ticket, a fog node at the right edge for what is not yet specified, and its Decisions so far and Out of scope beside the graph. Its header has **Write the Spec** once the Map is finished, and links the Requirements written from it.
_Avoid_: Map board, roadmap

**Write the Spec**:
The human's action on a finished Map (every Ticket Done or Cancelled) that creates a new Requirement with no grilling Stage, whose `map` Tag names the Map; its spec Stage works from the Map's Decisions so far and the resolutions they link. The Map stays as the record.
_Avoid_: Promote, convert the Map

**Spec view**:
The UI's view of a plain Spec, one that is neither a Requirement nor a Map: its title, **New Prototype** and its Tickets with their States. It opens from the Spec's name in the Ticket panel.
_Avoid_: Spec page, Spec board

**Charting**:
The first Ticket of a Map, which names its destination with the human and creates the first decision Tickets and their Blockers.
_Avoid_: Planning, setup

**Ticket**:
A unit of work within a Spec. Every Ticket belongs to exactly one Spec.
_Avoid_: Issue, task, card

**Prototype**:
A Ticket, on any Spec, that makes a cheap, throwaway artifact for the human to try and react to, so a decision can be made. Its work never lands. A Question answered "Prototype this" in a grilling or Charting Round becomes one, which blocks the asking Ticket until it is Done.
_Avoid_: Spike, mock-up, demo

**Stage**:
One of the three fixed steps that turn a Requirement's text into implementation Tickets (grilling, spec and tickets), each done as its own Ticket.
_Avoid_: Phase, step, definition Ticket

**Stage Guide**:
Code Central's instructions telling an agent what a Stage must produce, how to finish it and what to do when it can't carry on. Implementation Tickets, a Map's Tickets and Prototypes get one too, though they are not Stages.
_Avoid_: Prompt, playbook, skill

**State**:
The single position a Ticket occupies in the delivery workflow.
_Avoid_: Status

**Finished**:
A Ticket that is Done or Cancelled, or a Spec that has Tickets and all of them are finished.
_Avoid_: Closed, archived, complete

**Land**:
For a Ticket's accepted work to reach the Project's base, which makes the Ticket Done; whoever delivered it reports the landing. Under Merge + Push with CI Tracking on, a Ticket lands only once its pushed commit's CI result is passed, so Done means on the remote and green.
_Avoid_: Merge, ship, deliver, complete

**Landing Method**:
A Project's choice of how accepted work reaches its base. It is Merge (the Factory merges the branch into the base) or Branch (the Factory leaves the branch for the human to merge, and lands the Ticket when it sees the merge). Pull Request comes later.
_Avoid_: Completion option, delivery mode, finish mode

**Push**:
A Landing Method's option to send the work to a named git remote: the base after a Merge, or the Ticket's branch on accept under Branch. Under Merge + Push the Ticket lands only once the push succeeds, and with CI Tracking on, only once its CI result is passed as well.
_Avoid_: Publish, sync

**CI Tracking**:
A Project's opt-in, off by default, to have Code Central watch the CI of what the Factory pushes, gate landing on it and run its CI Rules. The human turns it on in the CI section of Project Options, naming the GitHub repository to watch.
_Avoid_: CI integration

**CI Provider**:
The pluggable part of CI Tracking that talks to one CI system, through its own command-line tool and login. GitHub Actions, through `gh`, is the only one.
_Avoid_: Integration, connector

**Watcher**:
A separate process, started and supervised by Code Central, that watches one external system for one Project, reaching it through `gh` and reporting over the HTTP API. Each kind is run while its Project setting is on, restarted with backoff when it crashes, and has its own status, Watching or an error with its fix, of which going into an error sends one Notification. The CI Watcher is the only kind so far.
_Avoid_: Daemon, poller

**CI Watcher**:
The kind of Watcher that watches one Project's CI through its CI Provider and reports what it sees over the HTTP API. Before CI Tracking turns on, Code Central runs it once in its **check mode**, which checks it can reach the repository through `gh` and answers with what is wrong and its fix, if anything is. Its status, **Watching** or an error with its fix, shows in the CI section of Project Options; going into an error sends one Notification. Rate-limited by GitHub, it is still Watching, until the time it may call again, and sends no Notification. The service starts one per Project with CI Tracking on, stops it when CI Tracking is turned off or the Project is archived, and restarts one that crashes with backoff, reporting it as crashed, restarted N times.
_Avoid_: CI daemon, CI Integration

**Pushed commit**:
The commit the Factory pushed for a Ticket, which it reports on the Ticket: the base after merging under Merge with Push, or the Ticket's branch on accept under Branch with Push. Its CI Runs give the Ticket's CI result.
_Avoid_: Push SHA

**CI Run**:
One workflow run the CI system started for a commit, tracked against the Ticket whose pushed commit it ran on, or against the CI Rule it fired. Kept for 30 days after it finishes.
_Avoid_: Job, build, pipeline

**CI result**:
A Ticket's running, passed or failed, worked out from the CI Runs of its latest pushed commit that aren't of Ignored workflows: running until they have all finished, failed if any failed, passed otherwise.
_Avoid_: Build status

**CI gate**:
What holds a pushed Ticket back from landing until its CI result is passed. It holds only while Merge, Push and CI Tracking all do, and lifts when any is turned off; the Ticket waits in its State, with no State of its own.
_Avoid_: CI check, CI hold, waiting-on-CI State

**Ignored workflows**:
A Project's list of workflow file names, such as `nightly.yml`, whose runs never count towards a CI result and never fire an "any workflow" CI Rule. They are still shown, marked ignored.
_Avoid_: Excluded workflows, skipped workflows

**CI Rule**:
A Project's standing rule, "when workflow X fails on branch Y, open a Ticket to fix it", whose Tickets go on a long-lived Spec of its own, as Draft or Ready.
_Avoid_: Trigger, watch, alert

**Fix Ticket**:
An ordinary implementation Ticket opened to fix a failed CI Run: by a CI Rule on its Spec, linking the run and naming its failed jobs, or by the human's New fix Ticket for a CI failed Ticket. While a CI Rule's fix Ticket is unfinished, further failures only comment on it.
_Avoid_: CI Ticket, failure Ticket

**Push remote**:
The git remote a Landing Method's Push sends work to, `origin` unless the human names another. The Factory reports its URL when it starts, from which the CI section of Project Options fills in the GitHub repository.
_Avoid_: Upstream, origin (for the concept)

**Blocker**:
A Ticket in the same Project that must reach Done before another Ticket can be claimed.
_Avoid_: Dependency

**Replacement**:
A Ticket named by a Cancelled Ticket as taking over its work.
_Avoid_: Successor

**Claim**:
An agent's exclusive hold on one Ticket, from taking it until the work leaves its hands. A Claim that has ended never comes back.
_Avoid_: Lock, assignment

**Claim ID**:
The identifier Code Central issues with a Claim, which proves the holder on every write that needs one, and which `claim` returns again to the session that holds it.
_Avoid_: Token, holder ID

**Resume Command**:
The command, given by whoever claims, that brings a Claim's session back from its working directory.
_Avoid_: Resume string, attach command

**Silent Claim**:
A Claim on a Ticket in progress with no recent activity, which may mean its session has died.
_Avoid_: Stale, stalled, lost

**Long Claim**:
A Claim on a Ticket in progress that is still active but hasn't moved for a long time, which may mean its agent is going round in circles.
_Avoid_: Looping, runaway, stuck

**Session Event**:
One thing a Claim's session did, as posted to Code Central: agent text, a tool call, a tool result, or a Factory log line.
_Avoid_: Log entry, message, activity

**Transcript**:
The Session Events of one Claim, in order, as Code Central holds them. It may be only the tail of the session.
_Avoid_: Log, conversation, stream

**Context Report**:
What agents load into context, measured from Claude Code's own session logs on the machine (not Transcripts): the size of tool results per kind of session (each Stage, implementation, and the review and other subagents), tool and operation, the largest results, the Specs and Tickets read more than once, and signs that a cut went too far, each with its change against a saved baseline.
It is not Usage: it reads Claude Code's logs on the machine, while Usage records what Code Central and the Factory see.
_Avoid_: Token report, usage report

**CI view**:
The view of the CI Runs in a Project, or in all Projects with CI Tracking on: the CI Watchers' statuses, then the runs still going and those finished in the last 7 days. Its read can also give only the CI Runs that count as running towards a Ticket's CI result.
_Avoid_: CI dashboard, build monitor

**Usage**:
The view of what agents have consumed in a Project, or in all Projects: their Code Central calls and the tokens their sessions used.
_Avoid_: Analytics dashboard, stats, metrics

**Context Size**:
How many tokens a session's model read for one response (input, cache write and cache read), which is how full its context window was.
_Avoid_: Context length, prompt size

**Actor**:
Who made a change: the human, an agent, or an Integration.
_Avoid_: User, caller

**Needs Me**:
The Tickets waiting on the human before they can move.
_Avoid_: Inbox, blocked

**To merge**:
The Needs Me reason of an Accepted Ticket under the Branch Landing Method, shown at once: the human merges its branch into the base, and the Factory lands it when it sees the merge.
_Avoid_: Ready to merge, awaiting merge

**CI failed**:
The Needs Me reason of an Accepted Ticket whose CI result is failed and that the human hasn't dealt with yet: send it back to fix, re-run the failed jobs, open a new fix Ticket, or land it anyway.
_Avoid_: Build broken, red

**Land anyway**:
The failure option by which the human accepts a CI failed Ticket's failed CI result, for its pushed commit only, so the Factory lands it on its next poll. Offered under Merge + Push only: under Branch + Push the human's merge decides.
_Avoid_: Override, force land

**Re-run failed jobs**:
The failure option by which the human asks the CI Watcher to re-run the failed jobs of a CI failed Ticket's failed CI Runs. It records a Re-run request on the pushed commit, and the Ticket's CI result reads running until the CI Watcher reports each run's new attempt. Offered under Merge + Push and Branch + Push.
_Avoid_: Retry, rebuild

**Re-run request**:
What Re-run failed jobs records on a pushed commit: the failed CI Runs, each at the attempt that failed, for the CI Watcher to re-run. It is answered when the CI Watcher reports a later attempt of each, and lapses after 10 minutes unanswered, say when GitHub refuses the re-run, so the Ticket is CI failed again.
_Avoid_: Rerun job, retry

**Send back to fix**:
The failure option by which the human sends a CI failed Ticket back to Ready with a note naming the failed run and jobs, so the next Claim fixes it on the same branch; its re-accept's newer pushed commit has its own CI result. Offered under Merge + Push and Branch + Push.
_Avoid_: Revert, reject

**New fix Ticket**:
The failure option by which the human opens a Ready Ticket on a CI failed Ticket's Spec, carrying the note, and accepts the failed CI result as Land anyway does, so the failed Ticket lands and the fix goes on top. Offered under Merge + Push and Branch + Push.
_Avoid_: Follow-up ticket, hotfix

**CI unavailable**:
The Needs Me reason of a Ticket that has waited 10 minutes or more on CI since its push, while the CI Watcher isn't watching or no CI Runs have been reported for its pushed commit.
_Avoid_: CI down, CI stuck

**Focus Mode**:
A view showing the human one Focus Item at a time from Needs Me, moving to the next as each is dealt with or skipped.
_Avoid_: Zen mode, queue view

**Focus Item**:
One thing in Needs Me the human can decide alone: one open Question, or one waiting Ticket.
_Avoid_: Action, card, task

**Activity Dashboard**:
What Focus Mode shows when nothing needs the human, unless the human has chosen All clear. It shows how many Tickets are Ready and In Progress, and lists the In Progress Tickets. When a Project in its scope that isn't Archived has CI Tracking on, a CI Running block also counts the CI Runs still going that count towards a Ticket's CI result, and a CI Running list below the In Progress Tickets lists them, or says "No CI Runs running". Each entry shows the run's workflow, Ticket, branch, short commit and how long it has been going, opens its Ticket in the Inbox and carries the run's GitHub link. A count pulses when it changes, and an entry pulses when its Ticket changes.
_Avoid_: Status board, overview, monitor

**Option**:
A global preference the human sets from the UI's Options view, kept by Code Central and applied at once.
_Avoid_: Setting (that word stays for `settings.yaml`), preference, config

**gh path**:
The Option naming where the `gh` command-line tool is, for when Code Central can't find it on its `PATH` or in the usual install places.
_Avoid_: gh binary setting

**Notification**:
An interruption sent to the human when an agent is stopped and waiting on them, as distinct from the Needs Me count.
_Avoid_: Alert, ping

**Question**:
A query an agent puts to the human on a Ticket, which stops the work on that Ticket until it is answered.
_Avoid_: Comment, prompt

**Round**:
The Questions an agent posts together on a Ticket, all of which must be answered or dismissed before the work continues.
_Avoid_: Batch

**Failure Report**:
An agent's permanent record of what broke on a Ticket, why, and what to fix, posted when it has no way forward.
_Avoid_: Error, incident

**Label**:
A free-form name on a Spec or Ticket, with no workflow meaning. Stage Labels, Wayfinder Labels and `prototype` are reserved: they mark a Ticket's Stage, a Map or a Map's Ticket type, and a Prototype. A Ticket carries at most one reserved type Label (a Stage Label, a Map Ticket type's Wayfinder Label or `prototype`), which chooses its Stage Guide.
_Avoid_: Category

**Wayfinder Label**:
A reserved `wayfinder:` Label marking a Map (`wayfinder:map`) or a Map's Ticket type (`chart`, `grilling`, `research`, `task`).
_Avoid_: Map Label, kind

**Node type**:
A Ticket's type on a Map, as Spec reads show it: its Wayfinder Label's type (`chart`, `grilling`, `research`, `task`), `prototype`, or `ticket` for a Ticket with neither.
_Avoid_: Ticket kind

**Tag**:
A free-form key and value on a Spec or Ticket, with no meaning to Code Central's workflow; the Factory reads the `guided` Tag on a Map, and the read side the `prototype`, `research` and `map` Tags.
_Avoid_: Label, attribute, property

**Import**:
A one-way, one-off migration of work into Code Central, after which Code Central owns it and the source is no longer maintained.
_Avoid_: Sync, Integration

**Integration**:
A plugin with an ongoing link to an external ticketing system, which owns the definition of the work it links while Code Central owns its delivery. CI Tracking is not an Integration: it owns no work, and only reports on CI.
_Avoid_: Import, connector, sync

**Link**:
The connection between a Spec or Ticket and the external item it mirrors through an Integration.
_Avoid_: Mapping, sync, reference
