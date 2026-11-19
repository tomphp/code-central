# Code Central

A local coordination point for AI agent-driven software delivery across many repositories: it holds the work and its state, and shows the human what needs them.

## Language

**Project**:
A body of work tracked here, with its Specs, Tickets and settings, spanning one or more Repositories, one of which is its Home Repository.
_Avoid_: workspace, monorepo

**Repository**:
One git repository that a Project's work changes, however many clones or worktrees of it exist. It keeps its own history, remote and Base, and is identified within its Project by its Repository Name. The Project keeps its Repositories in an order, the **landing order**: work lands in each Repository in that order, the Home Repository first by default, and the human can rearrange it. The landing order is Project-wide, never per Ticket.
_Avoid_: Repo, module, project

**Repository Name**:
The short, permanent name that identifies a Repository within its Project. It is set when the Repository is added and never changes. The Home Repository's name defaults to the Project Key. A Repository can be removed only when no unfinished Ticket has changed it, and finished Tickets keep showing its name.
_Avoid_: alias, slug

**Home Repository**:
The Project's one primary Repository, where the Factory runs and where the Project's file-based settings live. It is the only Repository a single-repository Project has, and a Ticket can change it like any other. It may be local-only, when its clone has no Push remote.
_Avoid_: Home Repo, main repository, meta repository

**Clone URL**:
The remote a Repository is cloned from and pushed to. Code Central works out the Repository's Code Host from it, and the CI section of Project Options fills in what to watch in the Repository from it. The Home Repository's Clone URL is its Push remote's URL as the Factory reports it, and is empty when its clone has no Push remote. Any other Repository needs a Clone URL, a local path or both. Any Repository without a Clone URL is local-only: it is never cloned, is adopted at its local path with no remote check, is skipped by Push, is refused under Pull Request, and is Not watched by CI until the human sets it by hand.
_Avoid_: remote, origin, repository URL

**Changed Repositories**:
The Repositories in which a Ticket's branch holds commits beyond the Base. The Factory reports them live while the Ticket is In Progress, and each finish settles them, replacing the earlier list. Nobody sets them by hand. Every Ticket records them, including those whose work never lands. A finish recorded before Repositories existed reads as the Home Repository's. A Spec's Changed Repositories are the union of its Tickets'.
_Avoid_: affected repos, target repos, scope

**Finish check**:
The Factory's comparison of a finish's record of the Changed Repositories with every Repository's branch, made after the turn that finished the Ticket and again before landing it, only while the Project has more than one Repository. A finish that leaves out a Repository whose branch holds work, or records a commit that isn't its branch's head, fails: the Ticket goes to Failed with a Failure Report on the finishing Claim. An Accepted Ticket that has landed somewhere can't fail it.
_Avoid_: finish validation, finish guard

**Home worktree**:
A Ticket's worktree in the Home Repository, `<home>.worktrees/<ticket-id>`. It is where the Ticket's session starts, its working directory. A Ticket has one worktree per Repository: at each Claim's start the Factory makes or reuses one in every Repository, whichever of them the Ticket goes on to change, and each other Repository's is beside its clone, at `<clone parent>/<Repository Name>.worktrees/<ticket-id>`. A Repository added mid-session gets its worktree at the next Claim.
_Avoid_: workspace, main worktree

**Archived Project**:
A Project put away by the human once all its work is finished, which takes no new work until it is brought back.
_Avoid_: Deleted, closed, inactive

**Project Key**:
The short, permanent name that identifies a Project to anything connecting to Code Central.
_Avoid_: Slug, Project ID

**Project Options**:
The page where the human sets a Project's settings, such as its Landing Method, CI Tracking, GitHub Issues, Models, Stage Guides, Auto-accept and Focus, with Auto-fix in its CI section, and its Repositories section, where the human adds and removes the Project's Repositories (the CLI does the same) and, in each Repository's row, sets its Own Landing, Push remote and CI choice. Unlike an Option, what it sets belongs to one Project.
_Avoid_: Project settings, preferences, config

**Store**:
Where Code Central keeps the Projects' records, Options, CI Runs and Usage. A service opens one Store, of one Store Kind.
_Avoid_: database, backend

**Store Kind**:
Files (YAML and Markdown files loaded into memory; the default), SQLite (one local database file) or Postgres (a database server that several services may share).
_Avoid_: store type, driver

**Store Move**:
Copying every record of one Store, Usage included, into another, empty Store of any Kind, offline, then switching the settings to it. The source is left untouched. It is not a Migration.
_Avoid_: migration, store migration

**Migration**:
A format bump that rewrites a Store to a new format version. It is not a Store Move, which copies a Store into another.

**Shared Store**:
A Postgres Store that several services on different machines open at once.

**Service Registration**:
A running service's record in the Store, holding its host, version and known format, renewed on a timer.

**Models**:
The section of Project Options where the human sets a model and effort for the Project default, for each kind of work (a Ticket's Stage Guide name) and for Chat. The Chat row is not a kind of work, and the Project default doesn't reach it.
_Avoid_: Model config, AI settings

**Model Choice**:
The model and effort a Factory session's turn runs with, worked out at every start and resume from, highest first: the Ticket's `model` or `effort` Tag, the Project's Models row for its kind of work, the Project default row, the Factory's `--model` or `--effort` flag, and Claude Code's own default. Model and effort are each worked out on their own. A model `claude` rejects, or an effort it doesn't accept, fails the Claim with a Failure Report naming where it was set.
_Avoid_: Model selection, model config

**Reported Models**:
The effort levels, model aliases and model IDs reported as accepted by the Project's `claude`, along with the Factory's own flags. Code Central keeps them across restarts and offers them in the model and effort pickers.
_Avoid_: Available models, model catalogue

**Factory**:
A program outside Code Central that claims Tickets and runs agents on them to deliver work.
_Avoid_: Controller, runner, orchestrator

**Turn**:
One run of claude the Factory starts, for a Claim or to answer a Chat Message. It ends done, failed, in an Outage, on a login failure, on a rejected model or effort, or stopped.
_Avoid_: run, process, session (a session spans many Turns)

**Outage**:
Trouble that is the service's or the account's limits, not the Ticket's: a usage or rate limit, overload, or a server or network error. A Claim's Turn cut off by one pauses the Factory and is resumed later. A Chat reply cut off by one fails, naming it.
_Avoid_: rate-limited (narrower), downtime

**Spec**:
The description of a piece of work, such as a feature or user story, delivered through one or more Tickets.
_Avoid_: Epic, feature, PRD

**All Tickets view**:
The Inbox's list of every Ticket in scope (`list=tickets`), of every kind and Stage, flat and newest first by creation, each row with its State chip, needs-you dot and kind label and naming its Spec. It follows the Project filter and reads only what the Inbox already loads. Its link counts the Tickets that are neither Done nor Cancelled.
_Avoid_: Ticket list, all Tickets page

**Board view**:
The view of the Tickets in scope as cards in one column per State, plus the Needs Me column while it's shown (`view=board`). Its link is in the Inbox's View section. It takes no action itself: a Ticket card opens its Focus popup or Ticket popup, and a Lane header its Spec popup, kept in the URL as `popup=`.
_Avoid_: Kanban, Ticket board

**Needs Me column**:
The Board view's column, shown or hidden in Columns ▾ and off unless the human turns it on, that sits after In Progress and holds the Ticket card of every Ticket in scope that needs the human, in place of its State's column, whatever State columns are hidden. Each card there also shows its State and Needs Me reason. It is kept in the URL as `needs=` and per browser. _Avoid_: meta-column, needs-me lane

**Sole Project**:
The only Project that isn't Archived. While there is one and the URL names no Project, the UI treats it as the selected Project: every view is scoped to it, IDs show as Short IDs, its Chat is offered and All Projects isn't offered. The UI works this out and never writes it into the URL. With no Project, or two or more Projects that aren't Archived, there is no Sole Project. A Project the URL names, an Archived one included, always wins.
_Avoid_: Only Project, Default Project

**Short ID**:
How the UI shows a Ticket's or Spec's ID while one Project is selected, or there is a Sole Project, its number alone as `#123`. With All Projects selected the full ID shows instead. Either way, clicking it copies the full ID.
_Avoid_: Ticket number, short key

**Long-lived Spec**:
A Spec labelled `long-lived`, which never finishes, such as a CI Rule's Spec, the GitHub Issues Spec, the CI Fixes Spec or the Small Changes Spec, so its Tickets land one by one, outside Land per Spec. Code Central labels the Specs it makes for CI Rules, GitHub Issues, CI fixes and Small Changes; the human may label or unlabel any Spec.
_Avoid_: Bucket Spec, standing Spec

**Section**:
A part of a Spec's body that starts at a heading and runs to the next heading at the same level or higher.
_Avoid_: Chapter, part, block

**Requirement**:
A Spec started from the human's free text and defined through Stages before it is built. Its free text is the Requirement's text. The human may save it as Draft, so every Stage Ticket is Draft and nothing is built until the human moves them to Ready.
_Avoid_: Prompt, feature request, idea

**Map**:
A Spec that finds the way to a loosely-defined destination through decision Tickets worked one at a time, each marked by a reserved Wayfinder Label. Its body is the index of decisions made, the fog not yet specified and what is out of scope. The human may save it as Draft, so its Charting Ticket is Draft until the human moves it to Ready.
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
The UI's view of a plain Spec, one that is neither a Requirement nor a Map: its title, **New Prototype**, **New Ticket** where the Spec takes one, its body, collapsed, with **Edit** until it is Finished, and its Tickets with their States. It opens from the Spec's name in the Ticket panel.
_Avoid_: Spec page, Spec board

**Edit**:
The human's action in the UI that changes the title or description of a Draft Ticket, or the title or body of a Spec that isn't Finished and isn't a Map. It decides only where the UI offers editing: agents and the CLI may edit a Spec or Ticket in any State.
_Avoid_: Modify, update, amend

**Charting**:
The first Ticket of a Map, which names its destination with the human and creates the first decision Tickets and their Blockers.
_Avoid_: Planning, setup

**Ticket**:
A unit of work within a Spec. Every Ticket belongs to exactly one Spec.
_Avoid_: Issue (that word means only GitHub's item, which an Incoming Issue mirrors), task, card

**Ticket History**:
A Ticket's Claims, Rounds, Comments, Moves, Edits and Failure Reports. The Store reads each part only when asked.

**Ticket head**:
A Ticket without its Ticket History, with the derived head fields the Store works out from that History on every write: when it entered its State, its live Claim, its latest Round and Failure Report, when it last went back to Ready, and when its Link's external state last changed. Lists and Needs Me read heads only.
_Avoid_: summary, stub

**Prototype**:
A Ticket, on any Spec, that makes a cheap, throwaway artifact for the human to try and react to, so a decision can be made. Its work never lands. A Question answered "Prototype this" in a grilling or Charting Round becomes one, which blocks the asking Ticket until it is Done.
_Avoid_: Spike, mock-up, demo

**Retro**:
A Ticket, on the Retros Spec, that looks back over the sessions that delivered a Finished Ticket or Spec, proposes changes to the agents' environment, and opens a Ticket for each change the human picks. Its work never lands. Its `retro-of` Tag names the Ticket or Spec it looks back on, and its description lists the Claims to read: every Claim on that work, with its session.
_Avoid_: Retrospective, post-mortem, review

**Retros Spec**:
The Long-lived Spec, one per Project, that Retros open on. Code Central creates it on first use, titled `Retros` and labelled `long-lived`, and finds it by its stored number.
_Avoid_: Retro Spec, Lessons Spec

**Retro button**:
The human's action on a Finished Ticket's or Finished Spec's view that opens a Retro on it, with an optional note.
_Avoid_: Run retro, review

**Small Change**:
The human's choice in **+ New ▾**, and also `submit_small_change`. It opens one implementation Ticket, Ready or, when the human saves it as Draft, Draft, from the human's free text on the Project's Small Changes Spec, with no Stages, so the Ticket is built and lands on its own. The first line is the title and the whole text the description.
_Avoid_: Quick change, task, hotfix

**Small Changes Spec**:
The Long-lived Spec, one per Project, that Small Changes open their Ticket on. Code Central creates it on first use, titled `Small Changes` and labelled `long-lived`, as it does the CI Fixes Spec, and finds it by its stored number, so retitling it breaks nothing.
_Avoid_: Changes Spec, Tasks Spec

**New Ticket**:
The human's action on a Spec view, Requirement view or Spec popup, and also `new_ticket`. It opens one implementation Ticket on that Spec from free text: the first line is the title and the whole text the description. It opens Ready, or Draft when saved as Draft, with no Stages. It is offered on any Spec but a Map or a Long-lived Spec, on a Requirement only once its Stages are finished, and on a Finished Spec too. Under Land per Spec its Ticket is cut from the Spec Branch and joins the Spec's pending batch, or a second batch if the Spec has landed.
_Avoid_: Add Ticket, follow-up Ticket, Request changes

**Straight to Spec**:
The human's choice, when submitting a Requirement, to have it start at the spec Stage, which works from the Requirement's text alone, with no grilling Stage. A Requirement with no grilling Stage that was not written from a Map shows its grilling Stage as skipped.
_Avoid_: Skip grilling, no-grill, fast track

**Implement**:
The human's choice, when accepting a Requirement's spec Stage, to build the Spec as one implementation Ticket, without the tickets Stage. It accepts the spec Stage, creates the Ticket and Cancels the tickets Stage with that Ticket as its Replacement.
_Avoid_: Skip tickets, fast path, direct build

**Split into Tickets**:
The human's other choice when accepting a Requirement's spec Stage. It accepts the spec Stage so the tickets Stage runs as usual.
_Avoid_: Break down, slice

**Stage**:
One of the three fixed steps that turn a Requirement's text into implementation Tickets (grilling, spec and tickets), each done as its own Ticket. The human may skip the grilling Stage by choosing Straight to Spec, and the tickets Stage by choosing Implement.
_Avoid_: Phase, step, definition Ticket

**Stage Guide**:
Code Central's instructions telling an agent what a Stage must produce, how to finish it and what to do when it can't carry on. Implementation Tickets, a Map's Tickets, Prototypes and Retros get one too, though they are not Stages. A Project can change what its guide says only through a named Project setting, such as Subagent Exploration or Model Assignment.
_Avoid_: Prompt, playbook, skill

**Model Assignment**:
A Project setting that has the tickets Stage Guide tell the agent to give each implementation Ticket it creates a `model` and `effort` Tag, chosen from the Project's Reported Models. Off unless the human turns it on.
_Avoid_: auto model assignment, model recommendation

**Subagent Exploration**:
A Project setting that has the implementation Stage Guide tell the agent to send reading across several files to a subagent and keep only its answer. On unless the human turns it off.
_Avoid_: Explore line, subagent mode

**State**:
The single position a Ticket occupies in the delivery workflow.
_Avoid_: Status

**Finished**:
A Ticket that is Done or Cancelled, or a Spec that has Tickets and all of them are finished.
_Avoid_: Closed, archived, complete

**Land**:
For a Ticket's accepted work to reach the base in every one of its Changed Repositories, which makes the Ticket Done; whoever delivered it reports the landing. It lands in each Repository by that Repository's Landing Policy's own test there, and records a landed commit per Repository; until it has landed in every one it stays Accepted, and is Partly landed once it has landed in some. Under Merge a Ticket lands in a Repository once its branch is merged into that Repository's base, and with Push once that base is pushed. Under Branch it lands in a Repository once its branch is merged there. Under Pull Request it lands in a Repository when the Code Host reports that Repository's pull request merged, at the host's merge commit, whatever the merge strategy. Under Merge + Push with CI Tracking on, it lands in a Repository only once its CI result there is passed, at the pushed commit that passed, so Done means on the remote and green at a commit containing its work. Under Land per Spec, a Spec-landed Ticket lands only with the rest of its Spec, once the Spec Branch is in the base in each Repository, and, with Push and CI Tracking, once its CI result is passed. Under Rebase, a Ticket's work on its Spec Branch is its replayed commits, and its branch is moved to them, so its head is what lands.
_Avoid_: Merge, ship, deliver, complete

**Landing Method**:
How accepted work reaches a base. The Project chooses it as the Default Landing, and a Repository may set its own as an Own Landing. Push skipping a Repository with no Clone URL is derived from the missing URL, not set. It is Merge (the Factory merges the branch into the base), Branch (the Factory leaves the branch for the human to merge, and lands the Ticket when it sees the merge) or Pull Request (the Factory pushes the branch and a pull request is opened on the Code Host). Merge and Branch have Push as an option, and Pull Request implies it; Land per Spec and Rebase apply under Merge and Pull Request, and have no effect under Branch.
_Avoid_: Completion option, delivery mode, finish mode

**Default Landing**:
The Project's Landing Method and Push option, followed by every Repository with no Own Landing. The Project may also name a Push remote, which is the default for its Repositories.
_Avoid_: global landing, project landing method

**Own Landing**:
A Repository's own Landing Method and Push option, set in place of the Default Landing. The Home Repository may have one.
_Avoid_: override, per-repo landing, repository landing method

**Landing Policy**:
The rules by which a Repository's Landing Method, Push, Land per Spec, Rebase and CI Tracking together decide how its accepted work lands: whether the CI gate holds, whether Land anyway is offered, whether a Ticket is To merge, whether the Pull Request Watcher runs, and the like. Every landing rule is one of its rules. A Repository's Landing Policy is its Own Landing, or else the Default Landing, with Push off when the Repository is local-only: there Merge + Push reads as Merge and Branch + Push as Branch. The Factory's landing steps in one Repository follow that Repository's Landing Policy; Project-wide rules hold when any Repository's policy calls for them.

**Landing capabilities**:
What the Project's landing view says Project Options offers for each Landing Method, by its Landing Policy, each as if that method were the Default Landing with every Own Landing unchanged: whether Push is offered (not under Pull Request, which implies it), whether Land per Spec can be changed (under Merge and Pull Request), whether Rebase can be changed (under Merge and Pull Request), whether the Code Host is shown (under Pull Request) and whether the PAT variable is used (while any Azure Repos Repository would be under Pull Request). Beside them, as saved: whether the Pull Request Watcher runs, the Default Landing named as a Repository Landing, and whether Push pushes nothing because every Repository is local-only. The Default Landing section and the Repositories section read them as the human switches the drafted method, and never branch on the method themselves.
_Avoid_: Landing flags, method options

**Pull Request**:
The third Landing Method: the Factory pushes the accepted branch to the Push remote, a Pull Request Watcher opens a pull request in each Changed Repository, into that Repository's own Base on its Code Host, and the Ticket lands when the hosts report them all merged. Push is implied. It applies per Repository, and needs a Clone URL on a supported Code Host: it is refused as a local-only Repository's Own Landing, and as the Default Landing while a local-only Repository follows it. A Home that loses its remote while under it can't be refused, as the Factory reports that: it is then local-only under Pull Request, which the Factory says once, and nothing lands there until it has another Landing or a remote. Under Land per Spec it opens one per Spec Branch per Changed Repository. Each pull request's body lists its sibling pull requests, if it has any, in the landing order, and the watcher keeps that list up to date; each one mentions the Spec's linked GitHub issue (`owner/repo#n`) with no closing keyword. The human decides the merge order, and Code Central doesn't enforce it. Each merge records that Repository's landed commit, at the host's merge commit, and the Ticket stays Partly landed until every pull request has merged.
_Avoid_: PR, merge request

**Code Host**:
The service that holds a Repository and its pull requests: GitHub (github.com, reached through `gh`) or Azure Repos (dev.azure.com, ssh.dev.azure.com or `*.visualstudio.com`, reached over the REST API with a PAT). It is per Repository: Code Central works it out from the Repository's Clone URL, shows it with each Repository in Project Options and doesn't let the human edit it; any other host is refused. One Project may mix GitHub and Azure Repos.
_Avoid_: Git provider, forge

**PAT variable**:
The Pull Request setting naming the environment variable that holds the Azure Repos PAT, `ADO_PAT` unless the Project names another. It is one Project-wide setting. Code Central never keeps the PAT; the setting is shown only when any Repository's Code Host is Azure Repos, and it may be the same variable as Azure Pipelines' if that token has both scopes.
_Avoid_: Token setting, secret

**Pull Request Watcher**:
The kind of Watcher that opens and watches the pull requests of one Project's Repositories under the Pull Request Landing Method and reports each one's link, state (open, conflicting, closed or merged) and, once merged, the host's merge commit over the HTTP API. The service runs one per Project that isn't Archived while any of its Repositories is under Pull Request, whether or not a Factory runs, and it watches the pull requests of every Repository under Pull Request and of no other; a failing Repository is named in its error, and it carries on with the others. Turning Pull Request on runs it once in its check mode against every Repository it brings under Pull Request, and adding a Repository that would be under Pull Request needs that Repository to pass the check too.
_Avoid_: PR bot, merge poller

**Pull request closed**:
The Needs Me reason of an Accepted Ticket whose pull request was closed or abandoned without merging, raised per pull request and naming its Repository: send it back (to Ready, with a note) or cancel it. Sending it back closes the Ticket's open sibling pull requests; Repositories that already merged keep their landed commit, and the Ticket's branch there is moved to the Base tip. Cancel is offered only while no sibling has merged. It clears if the pull request is reopened.
_Avoid_: PR abandoned

**Base**:
The branch that a Project's work is cut from and lands on, under every Landing Method. Each Repository has its own Base, `main` by default; the Home Repository's is the Project's Base, a Project setting.
_Avoid_: target branch, merge branch

**Base missing**:
The Needs Me reason of a Repository whose base exists neither locally nor on the Push remote; a local-only Repository's is never looked for on a remote, so it is missing when it isn't local. It is one Focus Item per Repository naming the Repository, the branch and the fix. Until a later poll finds the branch, the Factory starts no new Claims, and lands nothing whose Changed Repositories include that Repository; other work still lands. It sits beside Base diverged.

**Repository missing**:
The Needs Me reason of a Repository the Factory has no usable clone of: it couldn't clone it, or found something other than a clone with a matching Clone URL where the clone should be. It also covers a local-only Repository whose local path holds no clone of its own, and a Repository whose resolved path is another Repository's clone. It is one Focus Item per Repository naming the path, the Clone URL (left out when there is none) and what is wrong. The Factory never touches what it finds there, and starts no new Claims until a later poll finds the clone. It sits beside Base missing.

**Base diverged**:
The Needs Me reason of a Repository whose local base the Factory can't bring level with the remote base. Under Pull Request it means the local base holds commits the remote's doesn't, so the Factory can't fast-forward it. Under Merge + Push, where a local base that is only ahead is current, it means the local base and the remote's each hold commits the other lacks and the Factory couldn't move the local base onto the remote's: a rebuilt merge or a replayed commit conflicted, or an unpushed merge isn't one of the Factory's. It is one Focus Item per Repository naming the Repository and showing the local and remote commits and the fix. Until the human reconciles the base and the Factory's next poll brings it level cleanly, the Factory starts no new Claims, and lands nothing whose Changed Repositories include that Repository; other work still lands. A local-only Repository's base is never fetched or levelled, so it is never Base diverged.
_Avoid_: Out of sync, base conflict

**Land per Spec**:
A Merge and Pull Request option, on by default and kept whatever the method, which applies in a Repository by that Repository's Landing Policy, and by which a Spec's work gathers on one Spec Branch and lands in the base together, instead of Ticket by Ticket. It covers a Spec's implementation Tickets, not Stage Tickets, Map Tickets, Prototypes, Retros or the Tickets of a Long-lived Spec, and under Branch it has no effect. A Requirement is ready to land once its tickets Stage is finished and each of its Tickets is Accepted, Done or Cancelled; any other Spec once each of its Tickets is, so a Ticket added after its Spec has landed lands with a second batch. Under Pull Request the Spec gets one pull request instead of landing Ticket by Ticket. Turning it on, or taking a Spec's `long-lived` Label off, affects only Tickets accepted afterwards; turning it off, or labelling a Spec `long-lived`, lands the Spec's waiting Tickets together at once, ready or not.
_Avoid_: Spec landing, batch landing, release train

**Rebase**:
A Merge and Pull Request option, off by default and kept whatever the method, shown as "Rebase instead of merging", which applies in a Repository by that Repository's Landing Policy and is read at each landing step. It is Project-wide like Land per Spec: an Own Landing doesn't set it. With it on, the Factory never merges a branch into a Spec Branch: it replays an accepted Ticket's own commits, from its fork point, onto the Spec Branch's tip, keeping each one's author and message, and fast-forwards the Spec Branch, so the Spec Branch stays linear. When the Spec lands, the Factory first rebases the Spec Branch onto the base, then still makes the one "Merge spec" commit, so the base keeps one merge per landing. With Land per Spec off, a Ticket's branch is rebased onto the base and its "Merge" commit is kept. Conflicts are resolved by rebasing. Under Pull Request the Factory force-pushes with lease only the Spec Branches and Ticket branches it pushed and rewrote, never a base. Turning it on or off leaves commits already on a Spec Branch alone. Under Branch it has no effect.
_Avoid_: linear history, squash, fast-forward only

**Spec Branch**:
The local branch that a Spec's accepted implementation Tickets are merged into under Land per Spec: `spec/<spec id>`, or the value of the Spec's `branch` Tag when it has one. The name is the same locally, as pushed, and in every Repository. Each Repository has its own, cut from that Repository's Base, and each Ticket's branch in that Repository is cut from its tip; branch names are the same in every Repository. Once the Spec is ready to land, the Factory checks the Spec Branch of every one of the Spec's Changed Repositories against its base before any lands, then makes the same "Merge spec" commit in each, in the landing order. It is pushed only under Pull Request, where the Spec's one pull request is opened from it. Under Rebase, accepted Tickets are replayed onto it instead of merged into it, and it is rebased onto the base before it lands.
_Avoid_: Feature branch, integration branch, epic branch

**Spec merge Ticket**:
The Ready Ticket, "Merge the base into the Spec Branch of <spec id>", that Code Central opens, at the Factory's request, on a Spec whose Spec Branch conflicts with the base in any of its Changed Repositories, so an agent merges the base into the Spec Branch. With Rebase in force in any conflicting Repository it is titled "Rebase the Spec Branch of <spec id> onto the base" instead, and the agent rebases. There is one per Spec: it names each conflicting Repository with its files and both tips, and the Factory retries the Spec once it is accepted. While one is open, a later conflict is commented on it, and the same conflict again writes nothing. It carries a stored mark that only Code Central's open_spec_merge sets, which gives it the Merge results Auto-accept kind: a Ticket given either title any other way, or opened before the mark existed, is an ordinary Ticket and stays manual.
_Avoid_: Conflict Ticket, merge Ticket

**Waiting for the Spec**:
The status shown on an Accepted Ticket under Land per Spec whose work isn't yet in the base, because the rest of its Spec isn't done. Only its Changed Repositories where Land per Spec applies count, so one that changed only Repositories under Branch never waits for its Spec. It is not a State, and the Ticket isn't flagged in Needs Me as Accepted but not landed.
_Avoid_: Held, queued, staged

**Partly landed**:
The status shown on an Accepted Ticket that has landed in some of its Changed Repositories but not all. Under Land per Spec it is shown on a Spec's Tickets the same way. It is a status, not a State, like Waiting for the Spec. A Partly landed Ticket can't be cancelled, since what has landed stays landed: it is landed in the rest, or sent back.
_Avoid_: Half landed, partially merged, split

**Accepted but not landed**:
The Needs Me reason of an Accepted Ticket, under Merge, that has not landed after a while, so the Factory may be stuck. For a Partly landed Ticket it names the Repositories that have landed and those that haven't; Partly landed is not a reason of its own. It leaves out a Ticket that is Waiting for the Spec.
_Avoid_: Stuck, stale

**Push**:
A Landing Method's option to send the work to a named git remote, applied in each of the Ticket's Changed Repositories by that Repository's Landing Policy and Push remote: each one's base after a Merge, or the Ticket's branch on accept under Branch. Under Merge + Push the Ticket lands in a Repository only once that push succeeds, and with CI Tracking on, only once its CI result is passed as well, from that push or a later one containing its work. Under Land per Spec the Factory pushes each Repository's base once for a whole Spec, after merging that Repository's Spec Branch into it; under Merge and Branch the Spec Branches and Ticket branches are never pushed. Under Pull Request, Push is implied, with no checkbox: the Factory pushes the Ticket's branch in each Changed Repository on accept, or the Spec Branches under Land per Spec. In a Changed Repository with no Clone URL nothing is pushed: the Ticket lands there once its branch is merged into the base, and CI Tracking doesn't gate it there. So under Push "Done" means on the remote wherever there is one.
_Avoid_: Publish, sync

**CI Tracking**:
A Project's opt-in, off by default, to have Code Central watch the CI of what the Factory pushes, gate landing on it and run its CI Rules. It is one switch for the whole Project. The human turns it on in the CI section of Project Options, where each Repository has its own CI Provider and what to watch, a GitHub repository or an Azure DevOps project URL, filled in from its Clone URL, or is set to **Not watched**. The Home Repository's entry is a per-Repository CI choice like the others, and is no longer always watched.
_Avoid_: CI integration

**CI Provider**:
The pluggable part of CI Tracking that talks to one CI system, through that system's own command-line tool and login, or its API and a token from the environment. There are two: GitHub Actions, through `gh`, and Azure Pipelines. It is chosen per Repository, so one Project may use both.
_Avoid_: Integration, connector

**Not watched**:
The CI choice of a Repository whose CI CI Tracking never watches: it has no CI Provider, never gates landing, never raises a CI item and never counts towards a Ticket's CI result. Any Repository may be Not watched. A Repository whose Clone URL names no host a CI Provider is on, or that has no Clone URL, is Not watched until the human chooses one, so a local-only Home starts Not watched.
_Avoid_: CI off, untracked, ignored Repository

**Azure Pipelines**:
The CI Provider for Azure DevOps' CI. It reaches one Azure DevOps project, named by its URL, over Azure DevOps' API, using a PAT that the service's environment holds under a variable the Project names.
_Avoid_: ADO provider, Azure DevOps CI

**PAT environment variable**:
The Azure Pipelines setting that names the environment variable holding the Azure DevOps PAT, `ADO_PAT` unless the human names another. Code Central keeps the name, never the PAT.
_Avoid_: PAT setting, token variable

**Heartbeat**:
The signal a running Factory sends Code Central on each poll, naming its Project, itself and its host, its state, its note and when the next is due. It also carries the Factory's Plan Limits, when it has any. Code Central keeps only the latest of each Factory, in memory.
_Avoid_: ping, keepalive, check-in ("Ping" is already the session activity hook)

**Plan Limits**:
The Claude account's 5-hour limit and weekly limit, each as the percentage used and the time it resets, as a Factory last reported them from its Turns. They are account-wide, and go stale while the Factory is idle.
_Avoid_: usage, session limit, quota

**Limits popup**:
The popup fixed at the bottom centre of the UI that shows the selected Project's Plan Limits as a meter for each window, when the Show Limits popup Option is on.
_Avoid_: usage popup, meter popup, session popup

**Chat**:
Talking to Claude about a Project from the UI, outside any Ticket, through a floating icon and a pop-up.
_Avoid_: chatbot, assistant, console

**Conversation**:
One Chat session, from its first Chat Message until the human starts a new one. Each message resumes the same session. A Project has at most one current Conversation, held by one Factory.
_Avoid_: thread, chat session

**Chat Message**:
One message in a Conversation, from the human or from Claude.
_Avoid_: prompt, reply (as nouns for the record), Session Event

**Chat support**:
A Factory saying in its Heartbeat that it can run Conversations.
_Avoid_: chat capability, chat mode

**Chat worktree**:
A Repository's worktree for the Factory's Conversations: `<home>.worktrees/chat` in the Home Repository, beside the Home worktrees, where every Conversation runs, and `<clone parent>/<Repository Name>.worktrees/chat` beside each other Repository's clone, which a Conversation reaches as an added directory. A Conversation's first turn moves each one to its Repository's Base tip when it is clean; the Factory never removes them, so a Conversation's Resume Command keeps working.
_Avoid_: chat checkout, chat directory

**Factory Status**:
Whether a Project's Factories are **Online**, **Draining** (claiming nothing new while it finishes running turns) or **Offline** (no Heartbeat for three of its intervals, at least 90 seconds, or a stopping Heartbeat), as Code Central reads it from their Heartbeats. A Project's combined status is Online if any Factory is, else Draining if any is, else Offline, which is also a Project with no Heartbeat since Code Central started.
_Avoid_: health, presence, liveness

**Watcher**:
A separate process, started and supervised by Code Central, that watches one external system for one Project, reaching it through its CI Provider, `gh`, or Azure DevOps' API with a PAT, and reporting over the HTTP API. Each kind is run while its Project setting is on and its service holds the Project's Watcher Lease, restarted with backoff when it crashes, and has its own status, Watching or an error with its fix, of which going into an error sends one Notification; the status names the host it runs on, says Elsewhere, with the host, while another service holds the Lease, and Waiting once the Lease has lapsed with no service to take it. The kinds are the CI Watcher, the Issue Watcher, the Work Item Watcher and the Pull Request Watcher.
_Avoid_: Daemon, poller

**Watcher Lease**:
The claim one service holds on a Project's Watchers in a Shared Store, renewed while it runs and taken over when it expires.
_Avoid_: watcher lock

**CI Watcher**:
The kind of Watcher that watches one Project's CI, in every Repository that isn't Not watched, each through its own CI Provider, and reports what it sees over the HTTP API. A failing Repository is named in its error, and it carries on with the others. Before CI Tracking turns on, Code Central runs it once in its **check mode**, which checks it can reach what it watches in every watched Repository and answers with what is wrong and its fix, if anything is. Its status, **Watching** or an error with its fix, shows in the CI section of Project Options; going into an error sends one Notification. Rate-limited by its CI Provider, it is still Watching, until the time it may call again, and sends no Notification. The service starts one per Project with CI Tracking on, stops it when CI Tracking is turned off or the Project is archived, and restarts one that crashes with backoff, reporting it as crashed, restarted N times.
_Avoid_: CI daemon, CI Integration

**Pushed commit**:
The commit the Factory pushed for a Ticket in one of its Changed Repositories, which it reports on the Ticket per Repository: that Repository's base after merging under Merge with Push, or the Ticket's branch on accept under Branch with Push or Pull Request (the Spec Branch, for a Spec under Pull Request with Land per Spec). Under Merge with Push and CI Tracking it is also a later commit on that Repository's remote base that the Factory saw containing the Ticket's work, which it reports on each Ticket waiting on CI. A Ticket may have several in each Repository, one per push; their CI Runs give the Ticket's CI result in that Repository: under the CI gate those of every pushed commit there since the Ticket last moved back to Ready, otherwise its latest's. Several Tickets may share one within that Repository, and its CI failed, CI cancelled or CI unavailable is one Needs Me item listing them all.
_Avoid_: Push SHA

**CI Run**:
One run of a Workflow the CI system started for a commit in one Repository, tracked against the Ticket whose pushed commit it ran on, or against the CI Rule it fired. A Ticket-tracked run counts for every Ticket whose pushed commit it ran on, not only the one it was tracked against. On Azure DevOps under Pull Request, a build is matched by its pull request, since it ran on the host's merge commit. Kept for 30 days after it finishes.
_Avoid_: Job, build, pipeline

**Workflow**:
The named definition a CI Run runs, as its CI Provider names it: a workflow file such as `ci.yml` for GitHub Actions, or a pipeline's name for Azure Pipelines.
_Avoid_: pipeline, except for Azure Pipelines' own

**CI result**:
A Ticket's running, failed, cancelled or passed. A pushed commit's is worked out from its CI Runs that aren't of Ignored workflows, each at its latest attempt: running while any is unfinished, else failed if any failed or timed out, else cancelled if any was cancelled, else passed; a partially succeeded run counts as passed. A commit whose only runs are cancelled is cancelled, not unavailable, and a cancelled run that was re-run and succeeded counts as passed. A Ticket has one per Changed Repository that isn't Not watched. Under the CI gate a Ticket's CI result in a Repository is passed if any of its pushed commits there since it last moved back to Ready has passed, and otherwise its latest pushed commit's (ADR 0034); under Branch + Push and Pull Request it is always its latest pushed commit's, and stays informational. These combine into the Ticket's CI result as running, else failed, else cancelled, else passed. Under the CI gate each Repository lands on its own passed CI result, and the Ticket is Partly landed until all have. The Ticket shows each Repository's CI result and the pushed commit it comes from, and lands in that Repository at that commit.
_Avoid_: Build status

**CI gate**:
What holds a pushed Ticket back from landing in a Repository until its CI result there is passed, so it holds on running, failed and cancelled. It holds in a Repository by that Repository's Landing Policy, and neither a Not watched Repository nor a local-only one, where nothing is pushed, is gated. It holds only while Merge, Push and CI Tracking all do, and lifts when any is turned off; the Ticket waits in its State, with no State of its own. Under Land per Spec a Spec's waiting Tickets share one pushed commit per Repository, so one CI result in each Repository gates them all.
_Avoid_: CI check, CI hold, waiting-on-CI State

**Ignored workflows**:
A Project's list of workflow names, such as `nightly.yml` or a pipeline's name, whose runs never count towards a CI result and never fire an "any workflow" CI Rule. They are still shown, marked ignored.
_Avoid_: Excluded workflows, skipped workflows

**CI Rule**:
A Project's standing rule, "when Workflow X fails on branch Y, open a Ticket to fix it", fired only by a run that failed or timed out, never by a cancelled one, and whose Tickets go on a Long-lived Spec of its own, as Draft or Ready.
_Avoid_: Trigger, watch, alert

**Fix Ticket**:
An ordinary implementation Ticket opened to fix a failed CI Run: by a CI Rule on its Spec, linking the run and naming its failed jobs, never its cancelled ones, or by the human's New fix Ticket on a CI failed item, or by Auto-fix, on the CI Fixes Spec. While a CI Rule's fix Ticket is unfinished, further failures only comment on it.
_Avoid_: CI Ticket, failure Ticket

**Push remote**:
The git remote a Landing Method's Push sends work to, `origin` unless the human names another, for the Project (the default) or for a Repository. Its URL in the Home Repository, which is the Home Repository's Clone URL, is reported by the Factory when it starts, and is empty when the clone has no Push remote.
_Avoid_: Upstream, origin (for the concept)

**Blocker**:
A Ticket in the same Project that must reach Done before another Ticket can be claimed, which an Accepted Ticket waiting on CI doesn't until it lands. Under Land per Spec, a Blocker on the same Spec as the dependant is also cleared once it is Accepted and its work is merged into the Spec Branch. A Blocker that no longer blocks though it isn't Done is cleared, shown as "no longer blocking".
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
The command that brings a session back: a Claim's, given by whoever claims, from its working directory; or a Conversation's. A Conversation's Resume Command changes into the worktree itself, because the human runs it from anywhere. With several Repositories, the Factory's Resume Commands add the other Repositories' worktrees as directories, as its sessions do.
_Avoid_: Resume string, attach command

**Silent Claim**:
A Claim on a Ticket in progress with no recent activity, which may mean its session has died.
_Avoid_: Stale, stalled, lost

**Long Claim**:
A Claim on a Ticket in progress that is still active but hasn't moved for a long time, which may mean its agent is going round in circles.
_Avoid_: Looping, runaway, stuck

**Session Event**:
One thing a Claim's session did, as posted to Code Central: agent text, a tool call, a tool result, or a Factory log line. A Conversation's events are not a Claim's Transcript.
_Avoid_: Log entry, message, activity

**Transcript**:
The Session Events of one Claim, in order, as Code Central holds them. It may be only the tail of the session.
_Avoid_: Log, stream

**Context Report**:
What agents load into context, measured from Claude Code's own session logs on the machine (not Transcripts): the size of tool results per kind of session (each Stage, implementation, and the review and other subagents), tool and operation, the largest results, the Specs and Tickets read more than once, and signs that a cut went too far, each with its change against a saved baseline.
It is not Usage: it reads Claude Code's logs on the machine, while Usage records what Code Central and the Factory see.
_Avoid_: Token report, usage report

**CI view**:
The view of the CI Runs in a Project, or in all Projects with CI Tracking on: the CI Watchers' statuses, then the runs still going and those finished in the last 7 days. Its read can also give only the CI Runs that count as running towards a Ticket's CI result.
_Avoid_: CI dashboard, build monitor

**Usage**:
The view of what agents have consumed in a Project, or in all Projects: their Code Central calls and the tokens their sessions used. It shows its parts in four tabs, Token usage, Context Size, Code Central calls and Storage, in that order, with the open tab in the URL; Storage shows the Store, each Project's records, the service's memory and the Service Registrations, as they are now; a session opens in its Session popup. Its token kinds include `chat`. Subagents nest under their parent session. It is not Plan Limits and not the Limits popup.
_Avoid_: Analytics dashboard, stats, metrics

**Storage tab**:
The Usage page's tab that shows the Store Kind, its size, record counts, process memory and the registered services.

**Session popup**:
Usage's popup for one session (a Claim's main session or a Subagent): its Context Size response by response as a line chart, opened from Recent sessions. It is kept in the URL. A Subagent's popup links to its parent session's in place. It is not the Limits popup.
_Avoid_: session details, session modal, graph popup

**Subagent**:
A session that a Claim's session starts through the Agent tool. Usage records it as a session of its own under its parent session, named by its Agent Type and description.
_Avoid_: Child session, sidechain, task

**Agent Type**:
The kind of agent a Subagent was started as, as its Agent call names it, such as Explore or general-purpose.
_Avoid_: Subagent kind (Kind is already the Usage session kind)

**Context Size**:
How many tokens a session's model read for one response (input, cache write and cache read), which is how full its context window was.
_Avoid_: Context length, prompt size

**Actor**:
Who made a change: the human, an agent, or an Integration.
_Avoid_: User, caller

**Needs Me**:
The Tickets waiting on the human before they can move, a Project's Base diverged or Base missing, a Repository missing, and each pushed commit's CI failed, CI cancelled or CI unavailable, one item per commit listing the Tickets waiting on it.
_Avoid_: Inbox, blocked

**Needs Me count**:
The number of things in Needs Me, as the human sees it. Each item counts once, except that a Requirement's Stage Tickets count once in total, as their Requirement. It is given for all Projects and per Project.
_Avoid_: row count

**By Ticket**:
The default grouping of a State list: its Tickets alone, as a flat list. On the Board view, it shows the columns alone.
_Avoid_: Flat view, list mode

**By Spec**:
The grouping of a State list that shows the Specs with Tickets in it as Spec rows, each expanding to those Tickets, instead of the Tickets alone. On the Board view, it splits the columns into one lane per Spec, each lane headed by its Lane header.
_Avoid_: Spec-first view, tree view, Spec mode, swimlanes

**Show finished**:
The All Tickets view's checkbox, off by default, that brings back its Done and Cancelled Tickets; Accepted Tickets are always shown. It is kept in the URL as `finished=1` and per browser beside the grouping.
_Avoid_: Show done, history toggle

**Columns ▾**:
The Board view's menu that shows or hides each State's column and the Needs Me column. It is kept in the URL as `hide=` and per browser.
_Avoid_: Column filter, column picker

**Spec row**:
The one row the UI shows a Spec in, in the Requirements view, Needs Me and By Spec: ID, kind, title and needs-you dot, then the Stage strip for a Requirement or the State chips for any other Spec, then the headline. It expands to the Spec's Tickets.
_Avoid_: Spec card, Requirement row, bs-card

**Spec kind**:
What kind of Spec a Spec is, as its badge on a Spec row, a Lane header and a Ticket row's Spec line says: Requirement, Map, Long-lived or Spec, read in that order, so a Long-lived Requirement is a Requirement. A Spec row and a Lane header show no badge for a Requirement, whose Stage strip says it. The service says each Spec's kind; the UI only words it.
_Avoid_: Spec type, Spec badge

**Stage strip**:
A Requirement's progress on its Spec row and its view: a grilling segment (a Map for a Requirement written from a Map, skipped for one submitted Straight to Spec), a spec and a tickets segment, and Build, each done, skipped (a Cancelled Stage, passed over as a done one is), needs (waiting on the human), queued (waiting for a Factory), running (a Factory is on it) or waiting (behind an earlier segment). A Stage segment is done when its Ticket is Done and skipped when Cancelled; otherwise needs when its Ticket waits on the human, queued when it is the first open Stage and a Factory could claim it, waiting while Ready or Draft, and running otherwise. Build is done when it has Tickets, all Done and none waiting on the human; otherwise needs when any Build Ticket waits on the human, queued when no Stage is open and a Factory could claim one, running when no Stage is open and it has Tickets, and waiting otherwise. The current segment is the first not done or skipped, else Build. Its needs are every Ticket waiting on the human, Stage Tickets first in Stage order, then Build Tickets in claim order. The Requirement opens on the first of its needs, else the current Stage's Ticket, else its first unfinished Build Ticket, else its last Build Ticket, else its last Stage Ticket. The service works all of this out on a Requirement's view; the UI names the segments and words the headline.
_Avoid_: Progress bar, Stage bar

**Lane header**:
The bar across the Board view that heads a By Spec lane, on one line: ID, kind, title, needs-you dot and headline, then how many Tickets the lane shows. It opens its Spec popup; its chevron folds the lane.
_Avoid_: lane Spec row, lane bar

**Ticket row**:
The one row the UI lists a Ticket in, in the State lists, All Tickets, Needs Me, Drafts and a Spec row's Tickets: Short ID, Stage badge, title, State chip, Link and Waiting badges and needs-you dot, then the latest Session Event or the list's own detail. The whole row opens the Ticket.
_Avoid_: Ticket line, State row

**Ticket card**:
A Ticket on the Board view: Short ID, Stage badge, Blocked and needs-you dot, then the title, then its Spec's kind and title outside a lane. In the Needs Me column it also shows its State chip and Reason badge. It pulses when it joins its column (the Needs Me column too) or its Ticket changes. Clicking it opens its Focus popup or Ticket popup.
_Avoid_: Board card, Ticket tile

**Focus Pop-up**:
The Option that has the Focus popup open by itself, with every Focus Item in Needs Me in scope, on every view but Focus Mode, Options and Project Options, when a view loads with Focus Items or a new Focus Item arrives. A Needs Me reason its Project's Focus Kinds leave off neither opens it nor shows in it. Off unless the human turns it on.
_Avoid_: auto popup, needs-me popup, auto focus

**Focus popup**:
A popup showing Focus Items one at a time, each with its Focus badge, in the view Focus Mode also uses. It shows either a Board Ticket card's Ticket's Focus Items (its CI item's, for a Ticket listed in one), even of a reason its Project's Focus Kinds leave off, or, under Focus Pop-up, every Focus Item in the view's scope, with Skip and N left. When the Focus Item on screen is dealt with, it shows the same notes as Focus Mode: it moves on silently when the human acted on it there, and with a brief "Dealt with elsewhere" note over the next item when someone else did. It opens by itself only once no other popup is open and no text field has focus. It closes once none is left, whoever dealt with them, or, under Focus Pop-up, once only skipped ones are.
_Avoid_: needs-me popup, action popup

**Ticket popup**:
The Board view's popup for a Ticket card whose Ticket doesn't need the human. It shows its Ticket pane as the Inbox shows it, actions included.
_Avoid_: details popup, modal, quick view, peek

**Spec popup**:
The Board view's popup for a Lane header's Spec. It shows the Spec's pane as the Inbox shows it (Requirement, Map or Spec), actions included. Choosing one of its Tickets opens that Ticket's Focus popup or Ticket popup in its place.
_Avoid_: lane popup

**To merge**:
The Needs Me reason of an Accepted Ticket under the Branch or Pull Request Landing Method, shown at once: the human merges its branch into the base, and the Factory lands it when it sees the merge. Under Pull Request it links the pull request and says conflicting while the Code Host reports so. Under Branch, and under Pull Request without Land per Spec, the human may send the Ticket back to fix with a note. For a Spec's pull request under Land per Spec, they may open a New fix Ticket.
_Avoid_: Ready to merge, awaiting merge

**CI cancelled**:
The Needs Me reason of a pushed commit whose CI result is cancelled, listing the Accepted Tickets waiting on it that the human hasn't dealt with yet: re-run, land anyway or send back. It is one item per commit, named `Name@sha`, with no Ticket of its own, as CI failed is.
_Avoid_: CI aborted

**CI failed**:
The Needs Me reason of a pushed commit whose CI result is failed (a run failed or timed out; a cancelled run is CI cancelled instead), listing the Accepted Tickets waiting on it, across Specs, that the human hasn't dealt with yet: re-run the failed jobs, land them anyway, open a new fix Ticket, or send it back to fix. The item is left out of Needs Me while its fix Ticket is neither Done nor Cancelled, and while Auto-fix handles or covers it, and it shows the "Auto-fix stopped" line once Auto-fix has reached its limit; it returns if the fix Ticket is Cancelled while the commit is still failing. It is one item per commit, named `Name@sha` (its Repository's name and the commit), with no Ticket of its own, listing every Ticket that shares it, and each option acts on every listed Ticket, on that Repository's commit only: a listed Ticket's other Repositories keep their own CI result and land on their own green; send it back to fix is offered only when the item lists one Ticket that isn't Spec-landed. Each listed Ticket's view shows its CI result and links the item.
_Avoid_: Build broken, red

**Land anyway**:
The failure option by which the human accepts a CI failed or CI cancelled item's CI result, stored once on the Project for that pushed commit only, so the Factory lands every Ticket the item lists in that commit's Repository on its next poll. Their other Repositories keep their own CI result. Offered under Merge + Push only: under Branch + Push and under Pull Request the human's merge decides, and a local-only Repository lands once merged.
_Avoid_: Override, force land

**Re-run failed jobs**:
The failure option by which the human asks the CI Watcher to re-run the failed jobs of a CI failed or CI cancelled item's failed and cancelled CI Runs, once for the pushed commit, in its Repository only. It records a Re-run request on the Project for the commit, and every listed Ticket's CI result reads running until the CI Watcher reports each run's new attempt. Offered under Merge + Push and Branch + Push.
_Avoid_: Retry, rebuild

**Re-run request**:
What Re-run failed jobs records for a pushed commit: the failed and cancelled CI Runs, each at the attempt that failed, for the CI Watcher to re-run. It is answered when the CI Watcher reports a later attempt of each, and lapses after 10 minutes unanswered, say when the CI Provider refuses the re-run, so the commit is CI failed again.
_Avoid_: Rerun job, retry

**Send back to fix**:
The failure option by which the human sends a CI failed Ticket back to Ready with a note naming the failed run and jobs and their Repository, so the next Claim fixes it on the same branch; its re-accept's newer pushed commit in that Repository has its own CI result, and its other Repositories keep theirs. Offered under Merge + Push and Branch + Push, only when the item lists exactly one Ticket and that Ticket isn't Spec-landed. It is also a To merge option, not only a failure option: there the note says what to change.
_Avoid_: Revert, reject

**New fix Ticket**:
The failure option by which the human opens a Ready Ticket on the Project's CI Fixes Spec, carrying the note, which starts from the run summary and the Fix instructions, and stores it once on the commit's response. It doesn't accept the failed CI result: every Ticket the item lists stays Accepted, and lands in that commit's Repository when a later commit there containing the work passes CI, such as the fix's push. Its other Repositories keep their own CI result. Offered under Merge + Push and Branch + Push. It is also the To merge option on a Spec's lead Ticket under Pull Request + Land per Spec, its pull request open or closed: there it opens a Ready Ticket on the Spec itself, carrying the note, and the Ticket's accept is pushed to the Spec's pull request.
_Avoid_: Follow-up ticket, hotfix

**CI Fixes Spec**:
The Long-lived Spec, one per Project, that New fix Tickets, and Auto-fix, open their fix Ticket on. Code Central creates it on first use, titled `CI Fixes` and labelled `long-lived`, as it does a CI Rule's Spec.
_Avoid_: Fix Spec

**CI unavailable**:
The Needs Me reason of a pushed commit the CI gate has held Tickets on for 10 minutes or more since their push, while the CI Watcher isn't watching or no CI Runs have been reported for it. It is one item per commit, named `Name@sha`, with no Ticket of its own and no options, listing the Tickets waiting on it.
_Avoid_: CI down, CI stuck

**CI item**:
A pushed commit's CI failed, CI cancelled or CI unavailable as Needs Me shows it: one item per commit, named `Name@sha` (its Repository's name and the commit), with no Ticket of its own, listing the Tickets whose latest pushed commit it is and that wait on it, accepted earliest first, across Specs, with the failure options the human may choose, which act on all of them. Worked out on read, never stored; each listed Ticket's view links it.
_Avoid_: CI group, lead Ticket

**CI response**:
The human's Land anyway or Re-run request for one pushed commit, or the fix Ticket opened for it, by whoever opened it, stored once on the Project for that commit, so every Ticket whose latest pushed commit it is reads it.
_Avoid_: CI choice, override record

**Focus Mode**:
A view showing the human one Focus Item at a time from Needs Me, moving to the next as each is dealt with or skipped.
_Avoid_: Zen mode, queue view

**Focus Item**:
One thing in Needs Me, of a reason its Project's Focus Kinds leave on, that the human can decide alone: one open Question, one waiting Ticket, one pushed commit's CI failed, CI cancelled or CI unavailable, a Project's Base diverged or Base missing, or a Repository missing. It opens with a Focus badge naming its reason. Its Ticket IDs show as Short IDs. When it is one Ticket's, its title links to that Ticket in the Inbox, a Stage Ticket's in its Requirement; a CI item's title and an item with no Ticket have no link.
_Avoid_: Action, card, task

**Reason badge**:
The badge, a dot and the words, that names a Ticket's Needs Me reason in the words of the Focus section of Project Options. It is shown in the Ticket pane, on Ticket cards and in a Spec's Ticket list.
_Avoid_: reason label, reason dot

**Focus badge**:
The Reason badge at the top of a Focus Item, with the "Focus: " prefix ("Focus: <reason>").
_Avoid_: focus type, focus label

**Auto-accept badge**:
The badge in the In Review panel's heading, "Auto-accept: <kind>", that names the Ticket's Auto-accept kind in the words of the Auto-accept section of Project Options, or "never" for a Ticket with none.
_Avoid_: accept page type, accept label

**Focus Kinds**:
The Project setting naming which Needs Me reasons are Focus Items for that Project. It is set in the Focus section of Project Options, with every reason on by default. A reason left off stays in Needs Me, its count, the needs-you dots and Notifications.
_Avoid_: focus filter, muted kinds

**Activity Dashboard**:
What Focus Mode shows when nothing needs the human, unless the human has chosen All clear. The Inbox's Activity link, in its Admin section, also opens it in a view of its own, whatever needs the human, for the Project the Inbox has chosen (All Projects when none) and with a Project selector and a Back button. It shows how many Tickets are Ready and In Progress, and lists the In Progress Tickets, each entry showing its Ticket's Short ID. When a Project in its scope that isn't Archived has CI Tracking on, a CI Running block also counts the CI Runs still going that count towards a Ticket's CI result, and a CI Running list below the In Progress Tickets lists them, or says "No CI Runs running.". Each entry shows the run's workflow, Ticket (as its Short ID), branch, short commit, its status as a badge, as every CI Run display does, and how long it has been going, opens its Ticket in the Inbox and carries the run's GitHub link. A count pulses when it changes, and an entry pulses when its Ticket changes.
_Avoid_: Status board, overview, monitor

**Option**:
A global preference the human sets from the UI's Options view, kept by Code Central and applied at once.
_Avoid_: Setting (that word stays for `settings.yaml`), preference, config

**Show Limits popup**:
The Option that shows or hides the Limits popup. It is on unless the human turns it off, and applies at once in every tab.
_Avoid_: Show usage popup

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
An agent's permanent record of what broke on a Ticket, why, and what to fix, posted when it has no way forward. The Factory posts one too, on the finishing Claim, when a finish leaves out a Changed Repository or records a commit that isn't its branch's head.
_Avoid_: Error, incident

**Label**:
A free-form name on a Spec or Ticket, with no workflow meaning. Stage Labels, Wayfinder Labels, `prototype`, `retro` and `long-lived` are reserved: they mark a Ticket's Stage, a Map or a Map's Ticket type, a Prototype, a Retro, and a Long-lived Spec. `long-lived` goes only on a Spec. A Ticket carries at most one reserved type Label (a Stage Label, a Map Ticket type's Wayfinder Label, `prototype` or `retro`), which chooses its Stage Guide. `auto-accept` is also reserved, goes on a Spec or a Ticket, and is not a type Label, so it doesn't count toward that limit.
_Avoid_: Category

**Auto-accept**:
The human's accept carried out within `finish`, when the finished Ticket is auto-accept eligible. It is recorded as a Move by the human, marked with its cause (the Project setting, the Ticket's Label or the Spec's Label), not as a new Actor. Only the human can turn it on.
_Avoid_: Auto-approve, auto-merge, system accept

**Auto-accept kind**:
The kind of Ticket a Project's Auto-accept setting is switched on for: Grilling outcomes (`grilling`), Spec recommendation (`spec`), Tickets Stage (`tickets`), Implementation results (`implementation`) or Merge results (`merge`). It is worked out on read from what a Ticket carries, never stored. Prototypes and Map Tickets have none and are never auto-accepted. The In Review panel names it in the Auto-accept badge.
_Avoid_: Auto-accept type, category

**Auto-accept Label**:
The reserved `auto-accept` Label, which on a Ticket or its Spec makes the Ticket eligible whatever the Project's setting. Only the human may add or remove it. Adding it also accepts every eligible Ticket it now covers that is In Review; removing it changes nothing already accepted.
_Avoid_: Auto label, opt-in Label

**Auto-accept switch**:
The control on a Ticket's detail or a Spec's page by which the human adds or removes that item's Auto-accept Label. It reflects only that item's own Label. It is offered only where the item can be auto-accepted, and asks first when turning it on would accept Tickets already In Review.
_Avoid_: Auto-accept toggle, Label editor

**Auto-fix**:
The Project setting, off unless the human turns it on, that opens a fix Ticket on the CI Fixes Spec for a CI failed item on the human's behalf instead of showing it in Needs Me, up to the Auto-fix limit. It is set in the CI section of Project Options, only while CI Tracking is on, and only the human sets it. Turning CI Tracking off leaves the setting as it is, but it acts only while CI Tracking is on.
_Avoid_: Auto fix tickets, auto-heal

**Fix instructions**:
The Project's text appended to every fix Ticket's description after its run summary, whether Auto-fix or the human's New fix Ticket opened it. It reads as a default text until the human sets their own, and can't be empty.
_Avoid_: Prompt, custom message

**Auto-fix attempt**:
One fix Ticket that Auto-fix opened, numbered within its failure chain: 1 for a failure listing no auto fix Ticket, otherwise one more than the highest it lists.
_Avoid_: Retry

**Auto-fix limit**:
The most Auto-fix attempts per failure chain, 3 unless the human sets 1 to 10. Past it, the CI failed item comes back to the human.

**Auto-fix mark**:
What marks an auto fix Ticket, one Auto-fix opened: its Auto-fix attempt, the Repository and commit it was opened for, the Tickets its CI failed item listed, and the previous attempt's fix Ticket. Only Code Central sets it, and nothing clears it; a fix Ticket the human opens with New fix Ticket has none.
_Avoid_: Auto flag

**Run summary**:
The part of a fix Ticket's description, and of the Send back to fix note, that Code Central writes from a pushed commit's CI: each failed run (or cancelled run, for CI cancelled) linked with its Workflow and failed jobs, the `gh run view <id> --log-failed` commands, and the last passed commit in that Repository when there is one.
_Avoid_: Fix note, failure note

**Spec recommendation**:
The `implement` or `split` that a spec Stage's `finish` must carry. The Ticket keeps the latest one; the web UI highlights the matching button, and a spec Stage is auto-accept eligible only if its latest finish carried one.
_Avoid_: Verdict, suggestion

**Wayfinder Label**:
A reserved `wayfinder:` Label marking a Map (`wayfinder:map`) or a Map's Ticket type (`chart`, `grilling`, `research`, `task`).
_Avoid_: Map Label, kind

**Node type**:
A Ticket's type on a Map, as Spec reads show it: its Wayfinder Label's type (`chart`, `grilling`, `research`, `task`), `prototype`, or `ticket` for a Ticket with neither.
_Avoid_: Ticket kind

**Tag**:
A free-form key and value on a Spec or Ticket, with no meaning to Code Central's workflow; the Factory reads the `guided` Tag on a Map and the `model` and `effort` Tags on a Ticket, the `branch` Tag on a Spec, and the read side the `prototype`, `research` and `map` Tags. The tickets Stage sets `model` and `effort` on the Tickets it creates when Model Assignment is on. Code Central checks two Tags' values. On a Ticket, `effort` must be a valid effort level. On a Spec, `branch` must be a valid branch name, a value another unfinished Spec uses is refused, and any change is refused once one of the Spec's implementation Tickets has been claimed.
_Avoid_: Label, attribute, property

**Import**:
A one-way, one-off migration of work into Code Central, after which Code Central owns it and the source is no longer maintained.
_Avoid_: Sync, Integration

**Integration**:
A plugin with an ongoing link to an external ticketing system, which owns the definition of the work it links while Code Central owns its delivery. GitHub Issues is the first, run by the Issue Watcher, and Azure Boards the second, run by the Work Item Watcher. Each has its own Spec that its Incoming Issues arrive on, the Issues Spec or the Azure Boards Spec, and both may be on at once. CI Tracking is not an Integration: it owns no work, and only reports on CI.
_Avoid_: Import, connector, sync

**Link**:
The connection between a Spec or Ticket and the external item it mirrors through an Integration. A Link is owned, the external system owning the item's definition, or an Outbound Link, Code Central owning it.
_Avoid_: Mapping, sync, reference

**ID Link**:
A link in Markdown text whose target is a Spec's or a Ticket's full ID, such as `[code-central-706](code-central-706)`. In the UI it opens that Spec or Ticket in place. Not a **Link**, the connection to an external item through an Integration.
_Avoid_: Ticket link (it may name a Spec), reference, mention

**GitHub Issues**:
A Project's opt-in, off by default, to link the issues of a list of GitHub repositories through an Integration. Any `owner/repo` may be listed, and each Repository's GitHub remote is suggested. The human turns it on in the GitHub Issues section of Project Options, naming the repositories, an optional filter label, and whether New issues need me; turning it on first checks `gh` can triage each listed repository's issues, and the first time creates the Issues Spec. Turning it off leaves every Link as it is. It is inbound only: Code Central never opens an issue, and reports progress only on each linked issue. It is the first Integration.
_Avoid_: Issue sync, GitHub integration

**Issue Watcher**:
The kind of Watcher that polls the issues of each repository one Project's GitHub Issues lists, through `gh`, writing as the `integration` Actor, and posts Code Central's progress back to the issues. Before GitHub Issues turns on, Code Central runs it once in its check mode, which checks `gh` is found and logged in and that, for each listed repository, it can read the repository, the repository has issues on, and the login can triage them, answering with what is wrong and its fix. Its status, Watching or an error with its fix, shows in the GitHub Issues section of Project Options; one failing repository is an error naming that repository, while the others are still watched. Going into an error sends one Notification, and being rate-limited is still Watching and sends none. The service starts one per Project with GitHub Issues on, whether or not the Factory runs, stops it when GitHub Issues is turned off or the Project is archived, and restarts one that crashes with backoff, reporting it as crashed, restarted N times.
_Avoid_: Issue daemon, syncer

**Issues Spec**:
A Project's Long-lived Spec that the Incoming Issues from every listed repository arrive on, with no Link and no Stages. It is titled like "GitHub Issues: owner/repo" while one repository is listed, and "GitHub Issues" once more than one is. It is created the first time GitHub Issues is turned on, and its title follows the list unless the human retitles it.
_Avoid_: Inbox Spec, triage Spec

**Azure Boards**:
A Project's opt-in, off by default, to link the work items an Azure DevOps query returns through an Integration. The human turns it on in the Azure Boards section of Project Options, choosing a Saved query or a WIQL filter, the environment variable holding the PAT (the Project's CI PAT variable unless it names its own), whether New issues need me, Hide Code Central, and the Branch format; the settings of the mode not chosen are kept. Turning it on, or changing what it runs while on, first runs the Work Item Watcher's check mode, and the first time it creates the Azure Boards Spec. Turning it off leaves every Link as it is. It is inbound only, and never changes a work item's State. It is the second Integration.
_Avoid_: ADO integration, Azure DevOps Boards sync

**Work item**:
Azure DevOps' own tracked item, such as a Product Backlog Item, Bug or User Story, which is what Azure Boards links. Like issue for GitHub, it is reserved for the external item, never a Ticket.
_Avoid_: ADO ticket, Boards item

**Saved query**:
One of the two ways Azure Boards chooses its work items: an Azure DevOps saved query, named by its URL and built in Azure DevOps' own query editor. It must be a flat list; a tree or links query is refused.
_Avoid_: Stored query, query ID

**WIQL filter**:
The other way Azure Boards chooses its work items: a WIQL WHERE clause, without the keyword, run in the Azure DevOps project its URL names. Code Central wraps it as `SELECT [System.Id] FROM workitems WHERE …`, so `@Me` and `@project` work and the result is always a flat list.
_Avoid_: WIQL query, custom query

**Work Item Watcher**:
The kind of Watcher that polls one Project's Azure Boards query over the Azure DevOps REST API with the PAT, writing as the `integration` Actor: each work item it matches arrives as an Incoming Issue and follows the work item's edits, and one that leaves the results is out of scope, or deleted when Azure DevOps can't read it. It posts Code Central's progress back to the work items, and has a check mode as the Issue Watcher does.
_Avoid_: Boards poller, ADO watcher

**Azure Boards Spec**:
A Project's Long-lived Spec that Azure Boards' Incoming Issues arrive on, with no Link and no Stages, titled like "Azure Boards: org/project" after the organization and project the chosen mode runs in. It is created the first time Azure Boards is turned on, reused if it is turned off and on again, and its title follows the settings unless the human retitles it. It is the counterpart of the Issues Spec.
_Avoid_: Inbox Spec

**Incoming Issue**:
A linked Draft or Ready Ticket on the Issues Spec or the Azure Boards Spec whose Link isn't detached, mirroring one GitHub issue or one Azure Boards work item. The external system owns its definition, and the triage actions are offered on it.
_Avoid_: Issue Ticket, imported issue

**New issue**:
The Needs Me reason of an Incoming Issue awaiting triage: it arrived while New issues need me was on, the human hasn't triaged it yet, and it isn't closed externally, which takes over.
_Avoid_: Untriaged, new Ticket

**Awaiting triage**:
Whether an Incoming Issue still waits for the human's first triage action. Code Central sets it on an Incoming Issue that arrives while New issues need me is on, and a triage action clears it for good.
_Avoid_: Unread, pending

**Triage action**:
What the human may do with an Incoming Issue, from either Integration: Keep as Draft, To Requirement or Won't fix. Each is offered on every Incoming Issue, awaiting triage or not, in its Ticket panel, and on a New issue's row and Focus Item, with a note on what it does that follows the Integration the Incoming Issue came from: Won't fix's says it closes a GitHub issue, or leaves a work item's State for the human to move.
_Avoid_: Triage option, disposition

**New issues need me**:
A setting of GitHub Issues and, its own, of Azure Boards, off by default: when on, each new Incoming Issue from that Integration also shows in Needs Me and Focus Mode under New issue until the human triages it.
_Avoid_: Notify on new issues

**Hide Code Central**:
A setting of Azure Boards, off by default, that the human sets in the Azure Boards section of Project Options. While it is on, Write-back to a work item still posts every comment, but nothing Code Central adds names Code Central, its Specs or its Tickets. It writes no Status label, drops the line linking back, leaves Ticket IDs out of landed lines, hides its marker as an opaque hash, and replaces this Project's IDs and ID Links in the written text with the items' titles, outside code. Comments and tags already on a work item stay as they are. Other prose is posted as written.
_Avoid_: Anonymous mode, unbranded Write-back, stealth mode

**Branch format**:
An Integration's Project Option, empty by default, that To Requirement fills in with the linked item's ID to give the new Requirement's `branch` Tag, for example `feat/{id}`. `{id}` is its one placeholder, and it is required. Azure Boards has one for now.
_Avoid_: branch template, branch pattern

**Keep as Draft**:
The human's triage action that leaves an Incoming Issue as it is, a Draft as it arrived, and clears its awaiting triage, taking it out of Needs Me for good, writing nothing to its issue or work item.
_Avoid_: Park, snooze

**To Requirement**:
The human's triage action that makes a Requirement from an Incoming Issue, its title, then its body, then the human's optional extra context, moves the issue's or work item's Link to the Requirement's Spec as an Outbound Link, and Cancels the Incoming Issue with the grilling Stage Ticket as its Replacement. When the Integration's Branch format is set, it also sets the Requirement's `branch` Tag from it.
_Avoid_: Promote, accept

**Won't fix**:
The human's triage action that Cancels an Incoming Issue, with an optional reason, and closes its GitHub issue as not planned, posting the reason as a comment. On a work item it only posts the reason and sets the `code-central:wont-fix` tag: the work item's State is left for the human to move in Azure DevOps.
_Avoid_: Reject, decline

**Status label**:
The one `code-central:` label Code Central keeps on a linked GitHub issue to show where its work stands, such as `code-central:draft` on an Incoming Issue not yet triaged off the Issues Spec, `code-central:requirement` after To Requirement, `code-central:building` once the Requirement's Stages are all Done or Cancelled, `code-central:done` once its Spec is Finished with something landed, or `code-central:wont-fix` after Won't fix or once its Spec is Finished with nothing landed. The Issue Watcher creates it in the repository if it is missing. On a work item it is an Azure DevOps tag of the same name, and the Work Item Watcher replaces only `code-central:` tags, leaving the others. Azure DevOps' "tag" stays lowercase and external, never Code Central's Tag. No status tag is written to a work item while Hide Code Central is on, and tags already there are left alone.
_Avoid_: Tag, state label

**Write-back**:
What Code Central writes to a linked GitHub issue or work item, worked out per Integration: its comments, such as a Requirement's grilling resolution, its Spec's body and each Ticket as it lands, its Status label, and, for a GitHub issue only, closing it; a work item's State is never changed. Each Ticket's landed comment names its commit in every Changed Repository, as `owner/repo@sha` for a GitHub-hosted Repository and as `Name@sha`, by Repository Name, otherwise; the comment listing what landed once the Spec is Finished also lists the Spec's Changed Repositories. In a single-repository Project they read as before. Code Central works out from the work's state what is due on each issue, never from events, and the Integration's Watcher makes it so, posting each comment exactly once. Only the linked issue or work item is written to, never another. Nothing is ever written from Rounds, answers, Failure Reports, Transcripts or Claims. Under Hide Code Central, an Azure Boards Write-back carries no Status label, no link back and no IDs.
_Avoid_: Sync, mirroring

**Outbound Link**:
A Link on which Code Central owns the definition and only writes to the external item, never reading its edits. To Requirement turns an Incoming Issue's Link into one on the Requirement's Spec.
_Avoid_: One-way sync, write-only link
