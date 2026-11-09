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
The page where the human sets a Project's settings, such as its Landing Method. Unlike an Option, what it sets belongs to one Project.
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

**Charting**:
The first Ticket of a Map, which names its destination with the human and creates the first decision Tickets and their Blockers.
_Avoid_: Planning, setup

**Ticket**:
A unit of work within a Spec. Every Ticket belongs to exactly one Spec.
_Avoid_: Issue, task, card

**Prototype**:
A Ticket, on any Spec, that makes a cheap, throwaway artifact for the human to try and react to, so a decision can be made. Its work never lands.
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
For a Ticket's accepted work to reach the Project's base, which makes the Ticket Done; whoever delivered it reports the landing.
_Avoid_: Merge, ship, deliver, complete

**Landing Method**:
A Project's choice of how accepted work reaches its base. It is Merge (the Factory merges the branch into the base) or Branch (the Factory leaves the branch for the human to merge, and lands the Ticket when it sees the merge). Pull Request comes later.
_Avoid_: Completion option, delivery mode, finish mode

**Push**:
A Landing Method's option to send the work to a named git remote: the base after a Merge, or the Ticket's branch on accept under Branch.
_Avoid_: Publish, sync

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
_Avoid_: Token report, usage report

**Actor**:
Who made a change: the human, an agent, or an Integration.
_Avoid_: User, caller

**Needs Me**:
The Tickets waiting on the human before they can move.
_Avoid_: Inbox, blocked

**To merge**:
The Needs Me reason of an Accepted Ticket under the Branch Landing Method, shown at once: the human merges its branch into the base, and the Factory lands it when it sees the merge.
_Avoid_: Ready to merge, awaiting merge

**Focus Mode**:
A view showing the human one Focus Item at a time from Needs Me, moving to the next as each is dealt with or skipped.
_Avoid_: Zen mode, queue view

**Focus Item**:
One thing in Needs Me the human can decide alone: one open Question, or one waiting Ticket.
_Avoid_: Action, card, task

**Activity Dashboard**:
What Focus Mode shows instead of All clear when nothing needs the human and the human has chosen it. It shows how many Tickets are Ready and In Progress, and lists the In Progress Tickets. A count pulses when it changes, and an entry pulses when its Ticket changes.
_Avoid_: Status board, overview, monitor

**Option**:
A global preference the human sets from the UI's Options view, kept by Code Central and applied at once.
_Avoid_: Setting (that word stays for `settings.yaml`), preference, config

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
A free-form name on a Spec or Ticket, with no workflow meaning. Stage Labels, Wayfinder Labels and `prototype` are reserved: they mark a Ticket's Stage, a Map or a Map's Ticket type, and a Prototype.
_Avoid_: Category

**Wayfinder Label**:
A reserved `wayfinder:` Label marking a Map (`wayfinder:map`) or a Map's Ticket type (`chart`, `grilling`, `research`, `task`).
_Avoid_: Map Label, kind

**Tag**:
A free-form key and value on a Spec or Ticket, with no meaning to Code Central's workflow; the Factory reads the `guided` Tag on a Map, and the read side the `prototype`, `research` and `map` Tags.
_Avoid_: Label, attribute, property

**Import**:
A one-way, one-off migration of work into Code Central, after which Code Central owns it and the source is no longer maintained.
_Avoid_: Sync, Integration

**Integration**:
A plugin with an ongoing link to an external ticketing system, which owns the definition of the work it links while Code Central owns its delivery.
_Avoid_: Import, connector, sync

**Link**:
The connection between a Spec or Ticket and the external item it mirrors through an Integration.
_Avoid_: Mapping, sync, reference
