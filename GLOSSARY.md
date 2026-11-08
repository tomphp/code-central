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

**Factory**:
A program outside Code Central that claims Tickets and runs agents on them to deliver work.
_Avoid_: Controller, runner, orchestrator

**Spec**:
The description of a piece of work, such as a feature or user story, delivered through one or more Tickets.
_Avoid_: Epic, feature, PRD

**Requirement**:
A Spec started from the human's free text and defined through Stages before it is built. Its free text is the Requirement's text.
_Avoid_: Prompt, feature request, idea

**Ticket**:
A unit of work within a Spec. Every Ticket belongs to exactly one Spec.
_Avoid_: Issue, task, card

**Stage**:
One of the three fixed steps that turn a Requirement's text into implementation Tickets (grilling, spec and tickets), each done as its own Ticket.
_Avoid_: Phase, step, definition Ticket

**Stage Guide**:
Code Central's instructions telling an agent what a Stage must produce and how to finish it. Implementation Tickets get one too.
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
The identifier Code Central issues with a Claim, which proves the holder on every write that needs one.
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

**Actor**:
Who made a change: the human, an agent, or an Integration.
_Avoid_: User, caller

**Needs Me**:
The Tickets waiting on the human before they can move.
_Avoid_: Inbox, blocked

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
A free-form name on a Spec or Ticket, with no workflow meaning. Stage Labels are reserved, and mark a Ticket's Stage.
_Avoid_: Category

**Tag**:
A free-form key and value on a Spec or Ticket, with no workflow meaning.
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
