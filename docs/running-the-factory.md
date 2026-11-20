# Running the Factory

This guide takes one Requirement through every Stage with `code-central-factory`, from submitting it to seeing its implementation land. It follows the same Requirement as [Working with Claude Code](working-with-claude-code.md), on `~/src/acme-shop` set up as in [Setting up a Project](setting-up-a-project.md), so you can compare the two ways. The prompts and output shown are copied from a real run, cut with `…` where they are long. The agents' wording, their Questions and how many implementation Tickets they make will differ from run to run; the lines the Factory prints, the States and the buttons won't.

The run started from a freshly set up `acme-shop`, so its IDs start at `acme-shop-1` again. If you've already worked the Requirement by hand, yours will be higher.

The Factory is a program you leave running in a terminal. It claims the Project's Ready Tickets, runs one `claude -p` session per Claim, and resumes a session when you answer its Round. Once you accept an implementation Ticket, it lands it by each of its Repositories' Landing, the Project's Landing Method unless a Repository has its own: under **Merge**, the default, it merges the branch into the base and lands it; under **Branch** it leaves the branch for you to merge, and lands the Ticket once you have (see [How accepted work lands](#how-accepted-work-lands)). What's left to you is what a Factory never does:

| The Factory | You |
| --- | --- |
| claims each Stage's Ticket, and each implementation Ticket once its Blockers are Done | submit the Requirement |
| starts the Stage's skill in a session of its own, in a worktree of its own | answer each Round in the UI |
| resumes the session when you answer its Round | review each finished Ticket, and accept or reject it |
| under Merge, merges an accepted implementation Ticket's branch into the base, pushes the base with Push on, and lands it | under Branch, merge each accepted Ticket's branch into the base |
| under Branch, pushes an accepted Ticket's branch with Push on, and lands the Ticket once you have merged its branch | stop the Factory |

Compared with working by hand, you type no prompts and run no sessions, and you don't land; under Merge you don't merge either.

## Working a Requirement

1. **Start the Factory in the repository.** `code-central-factory` comes in the same release archive as `code-central` (or `make build` builds it next to it, from source). Put it on your `PATH` beside `code-central`, as in [Running Code Central as a service](running-as-a-service.md). Then, in a terminal you can leave open:

   ```console
   $ cd ~/src/acme-shop
   $ code-central-factory
   base: main
   ```

   That one line is all it prints at the start. It is now running, and looks for work every 30 seconds. Keep this terminal open for every step below: each line it prints later is one thing it did.

   It refuses to start, and says why, if something it needs is missing. For example, every Claim's worktree starts from the base, so its sessions need `.mcp.json` committed there:

   ```console
   $ code-central-factory
   code-central-factory: refusing to start: .mcp.json isn't committed on main: every worktree starts from main, so its sessions need it; commit it first
   ```

   Step 3 of [A brand new repository](setting-up-a-project.md#a-brand-new-repository) commits it.

   The base, the branch work starts from and lands on, is the Project's **Base branch** setting in Project Options (`main` by default), read again on every poll. The Factory doesn't care which branch is checked out, or whether HEAD is detached. When you change it, the Factory prints `base: release, was main` and applies the new base to every step not yet taken: an Accepted Ticket or waiting Spec Branch not yet merged merges into it, and so does a branch in progress. Work already merged into the old base finishes there (its push and CI), an open pull request keeps its target, and a Ticket waiting under Branch is watched on the new base. The Factory remembers where it merged only while it runs. `code-central-factory -h` lists its flags. These are the ones that matter:

   | Flag | What it does |
   | --- | --- |
   | `--parallel <n>` | The most `claude` turns running at once. The default is 3. |
   | `--model <model>` | The model for every session it starts or resumes, such as `--model opus`, unless the Ticket sets one. The default is Claude Code's own. See [Model and effort](#model-and-effort). |
   | `--effort <level>` | The effort for every session it starts or resumes, such as `--effort high`, unless the Ticket sets one. The default is Claude Code's own. It must be a level `claude --help` lists; see [What `claude` accepts](#what-claude-accepts). |
   | `--bypass-permissions` | Runs every session in `bypassPermissions` mode, with no permission checks. It prints a warning line at the start. |
   | `--no-chat` | Turns off Chat support. The Factory reports `chat: true` in every Heartbeat by default, saying it can run Conversations for the Project's Chat; with `--no-chat` it reports `chat: false`. |
   | `-- <Claude Args>` | Everything after `--` is `claude` flags the Factory passes, as given, to every start and resume, such as `-- --chrome`. See [Claude Args](#claude-args). |

   Without `--bypass-permissions`, every session runs in auto mode with no permission prompts. Anything auto mode won't allow is denied and the turn carries on without it. If a turn ends on denied calls, the Factory posts a Round asking you about each one, and answering it lets the session try again. Allow rules for the repository go in its committed `.claude/settings.json`.

   The first Ctrl-C drains: the Factory claims, resumes and lands nothing more (except a claim whose answer was lost, which it still claims again), lets the running turns finish, then exits. A second Ctrl-C stops the running turns. Either way the Claims are kept, and the Factory picks them up when you start it again. Step 8 shows it.

   The Factory keeps a request open to Code Central and starts its poll at once when work appears: a new or approved Ticket, an Accept or Reject, answered Questions. It also polls at once when a turn ends, to fill the freed slot, but a Ticket whose session gave up waits for the next timed poll rather than being claimed again straight away. `--poll <duration>` (default 30s, at least 5s) remains the fallback for what Code Central can't see happen, such as CI results and merged Pull Requests, and sets the Heartbeat cadence.

   Check: the Factory is still running, and its only line is `base: main`.

2. **Submit the Requirement from the UI.** Open <http://localhost:7733>, click **+ New**, type the same Requirement's text and click **Submit requirement**:

   ```text
   Format prices for display

   Add a Go function that turns a price in pence into the string a customer sees, so 1999 shows as £19.99 and 5 shows as £0.05.
   ```

   The UI opens the Requirement, `acme-shop-1 Format prices for display`, with its grilling Ticket, `acme-shop-2`, in Ready and *Waiting for a Factory to claim Grilling. Nothing needs you yet.* The Factory claims it at once:

   ```text
   acme-shop-2 claimed (grilling): "Grilling: Format prices for display" in Spec acme-shop-1; worktree /Users/you/src/acme-shop.worktrees/acme-shop-2; resume: claude --resume 37f7978d-a171-4aee-b737-220845e8b7a8
   ```

   If Code Central can't be reached, or its answer is cut off, the claim may still have gone through, so the Factory claims the same Ticket again at once, up to three more times, a second apart. Code Central gives back the same Claim, so the Ticket is never left In Progress under a Claim nobody holds. The Factory prints a line when the answer is lost and another when claiming again recovers it:

   ```text
   acme-shop-2: claim: the answer was lost (<the error>): claiming it again
   acme-shop-2: claim: the answer came back on claiming it again
   ```

   Each Claim gets a worktree of its own under `~/src/acme-shop.worktrees`, named after the Ticket. A Stage's worktree is read-only and detached at the base's tip; the Stages write to Code Central, not to the repository.

   The session's prompt is the Stage's skill and the Ticket, `/grilling acme-shop-2`. The rest of what the session needs, its Ticket and Claim, how the Factory runs it and the Stage Guide, goes once into its system prompt (`--append-system-prompt`), which every resume passes again so a session keeps it after a compaction. The guide is the Project's own, built by Code Central and returned by `claim`, so the Factory needs no flag for what it says: the Project's [Stage Guides](setting-up-a-project.md#stage-guides) settings decide it. Only when an older service's `claim` returns no guide does the Factory ask for it with `get_stage_guide`, passing its Project.

   Check: `acme-shop-2` is In Progress, and the Requirement's row reads *Grilling: a Factory is on it*. Nothing needs you yet.

   To skip grilling, tick **Straight to Spec (no grilling)** in the dialog before you submit. The line at the foot changes to say it creates only the Spec and Tickets Stages, and the new Requirement opens with Grilling shown as skipped and Spec Ready for a Factory. The checkbox is off by default and only the New requirement dialog has it.

   **Map**, in the **▾** menu beside **+ New**, starts a Map from the same dialog instead. Its one Ready Ticket, *Charting: …*, is claimed at once like a grilling Ticket, in a read-only Stage worktree, and grills you on the Map's destination through Rounds. The Map's row in the **Requirements** view counts its decisions, such as *0/1 decided*, in place of the Stage strip. **Small Change**, the third item, opens one Ready Ticket on the Project's `Small Changes` Spec, which a Factory builds like any implementation Ticket. The same **+ New ▾** is in Focus Mode's header, beside **Exit**, and submitting there keeps you in Focus Mode, on the item you were on, with a brief "Requirement added", "Map added" or "Small Change added" note.

   **Small Change**, the third item in the **▾** menu, opens one Ready Ticket from your text on the Project's Small Changes Spec, with no Stages, so there is no grilling to wait for. The Factory claims it at once like any Ticket, with the implementation Stage Guide, in a worktree of its own on a branch named after it, and it lands on its own once you accept it, outside Land per Spec. From a terminal, `code-central submit-small-change --text "…"` (or `submit_small_change` over MCP) does the same, and prints the Ticket:

   ```console
   $ code-central submit-small-change --text "Fix the typo in the checkout button"
   {
     "id": "acme-shop-2",
     "state": "Ready"
   …
   }
   ```


3. **Watch the session in the Claim's Live section.** In the Ticket detail, under the description, each Claim has a line and a **Live** section. The line says when the Claim started, its last activity, the latest thing its session did and the session's ID. While the Ticket is In Progress, Live is open and follows the session's Transcript as it goes: the Factory's own lines, marked `FACTORY`, each tool call on one line with its result collapsed beneath it, and the agent's text:

   ```text
   Claim 1 · started 1m ago · last activity 1m ago · I've posted Round 1 on acme-shop-2 with six questions, each with a recommended answer. The Requirement asks for a Go fu… · 1m ago · session 37f7978d-a171-4aee-b737-220845e8b7a8
   ▾ Live
   FACTORY acme-shop-2 claimed (grilling): "Grilling: Format prices for display" in Spec acme-shop-1; worktree …   08:01:35
   Skill: mattpocock-skills:domain-modeling                                                                     08:01:39
   mcp__code-central__get_ticket: code-central get_ticket acme-shop-2                                          08:01:41
   …
   mcp__code-central__post_round: code-central post_round acme-shop-2                                          08:02:07
   Bash: code-central wait acme-shop-2                                                                          08:02:10

   I've posted Round 1 on acme-shop-2 with six questions, each with a recommended answer. …
   ```

   Click **Live** to close it, or to open it on a Claim that has ended. Every Ticket the Factory works has one, for every Stage and every implementation Ticket. To enter a session yourself, run the Claim's Resume Command (the `resume:` part of the Factory's line) in its worktree. Don't do it while the Factory has a turn running on it: the two would write into one session.

   The Factory also posts each model response's token usage and model to the service, so its sessions appear in [Usage](running-as-a-service.md#usage)'s Context Size and token figures, each with the models it used. A response Claude Code made up itself, whose model is `<synthetic>`, is posted with its tokens but no model. The main session counts under its Stage (`grilling`, `spec`, `tickets`) or `implementation`; each subagent is a session of its own, counted as `review` when its Agent call is described `Spec review …` or `Standards review …` and as `subagent` otherwise. A session resumed after you answer a Round adds to the same Usage session. Sessions you run by hand post nothing, so they aren't counted there, though their Code Central calls are. Like the Transcript, posting never holds up or fails a turn: a post the service can't take, such as an older service's `404`, is dropped.

   Check: the Live section shows the session's calls as they happen.

4. **Answer the grilling Round in the UI.** The session posts its Questions as a Round, not in a terminal:

   ```text
   Round 1  6 of 6 open                                     Accept all recommended

   Q1 Negative amounts
   The Requirement covers 1999 → £19.99 and 5 → £0.05, but not negative input (a refund, a discount line, a bad value).
   …
   Q2 Thousands separators
   Q3 Whole pounds keep the pence
   Q4 GBP only
   Q5 A Price type in pence
   Q6 Go module and package
   ```

   Answer it as in step 4 of [Working with Claude Code](working-with-claude-code.md#working-a-requirement): each Question has the agent's recommended answer as a draft, which you can edit and **Answer**, or **Dismiss**. In this run, **Accept all recommended** answered all six.

   There is nothing to tell a session. Here the session had left `code-central wait` running, so it picked the answers up itself. If its turn had ended instead, the Factory would resume it with the answers at once, and print a line such as `acme-shop-2 resumed (grilling): "Grilling: Format prices for display" with the answers to Round 1`. If the answers raise new Questions, another Round comes. Answer it the same way.

   Check: **Needs Me** is back to 0, and the Factory prints the turn's end:

   ```text
   acme-shop-2 turn ended (grilling): "Grilling: Format prices for display": exit 0, success (completed); now In Review; ran 2m21s
   ```

5. **Review and accept the grilling Stage.** `acme-shop-2` is In Review, with the session's summary in *Review the finished work*, beside an *Auto-accept: Grilling outcomes* badge:

   ```text
   Grilling took one Round of six Questions, and every recommendation was accepted. The full write-up is in the resolution comment on this Ticket. The Spec is retitled "Display string for a Price in pence", and its body is unchanged.

   Key decisions:
   - A type Price int64 in pence, whose String() method gives the display price, in package price under a new acme-shop module.
   …
   ```

   Read it, and the resolution comment in the Ticket's history, then click **Accept**. To send it back, write what to change in *What to change (required to reject)* and click **Reject**: the Ticket goes back to Ready, and the Factory claims it again at once. Your note is kept as a comment on the Ticket, for the new session to work from.

   The Factory removes the Ticket's worktree and claims the next Stage:

   ```text
   acme-shop-2: Done: removed its worktree /Users/you/src/acme-shop.worktrees/acme-shop-2
   acme-shop-3 claimed (spec): "Spec: Format prices for display" in Spec acme-shop-1; worktree /Users/you/src/acme-shop.worktrees/acme-shop-3; resume: claude --resume 58e74fca-2723-46df-a89a-8a4f1b3fa5fd
   ```

   Check: the strip's Grilling segment is done, and the Requirement's row reads *Spec: a Factory is on it*.

6. **Review and accept the spec and tickets Stages.** Each comes to In Review without you doing anything. A session posts a Round only if it has a Question the earlier Stages didn't settle. Neither session did in this run.

   ```text
   acme-shop-3 turn ended (spec): "Spec: Format prices for display": exit 0, success (completed); now In Review; ran 57s
   ```

   ```text
   I wrote the full body of Spec acme-shop-1 ("Display string for a Price in pence") with the seven template sections, from the Requirement and the resolution of grilling Ticket acme-shop-2. No repository files changed.
   …
   Please check: that treating math.MinInt64 as in range (rather than out of range) and the 1.27 go directive are acceptable.
   ```

   Check the Spec's body on `acme-shop-1`. The spec Stage's review offers **Split into Tickets** and **Implement**. The agent's `finish` recommended one of them by the Spec's size, so that button is highlighted and marked *Recommended*, and the summary ends with a one-line reason. **Implement** builds the Spec as one implementation Ticket and cancels the tickets Stage, which the strip shows as skipped, so the Factory goes straight to building it. Here, click **Split into Tickets** on `acme-shop-3`. The Factory claims the tickets Stage, `acme-shop-4`:

   ```text
   acme-shop-4 turn ended (tickets): "Tickets: Format prices for display": exit 0, success (completed); now In Review; ran 42s
   ```

   ```text
   Sliced Spec acme-shop-1 into two Ready implementation Tickets, a linear chain:

   acme-shop-5: Price prints its Display price for zero and positive amounts. Blocked by: acme-shop-4. The tracer bullet. …
   acme-shop-6: Negative Prices print with a leading minus, down to math.MinInt64. Blocked by: acme-shop-4, acme-shop-5. …
   ```

   **Accept** `acme-shop-4`.

   Check: Grilling, Spec and Tickets are done on the strip, and Build shows *0/2*. This run made two implementation Tickets where the Claude Code run made one; yours may differ again.

7. **Accept each implementation Ticket and see it land.** The Factory claims `acme-shop-5` as soon as its Blocker is Done. Unlike a Stage, it works on a branch named after the Ticket, in its own worktree, and commits there:

   ```text
   acme-shop-5 claimed (implementation): "Price prints its Display price for zero and positive amounts" in Spec acme-shop-1; worktree /Users/you/src/acme-shop.worktrees/acme-shop-5; resume: claude --resume d6b03b7b-6542-44f6-8b10-a51c756c8237
   acme-shop-5 turn ended (implementation): "Price prints its Display price for zero and positive amounts": exit 0, success (completed); now In Review; ran 2m0s
   ```

   Its summary names the branch and head commit, the acceptance criteria met, the test result, the code review's outcome (what the Standards and Spec axes found and what was fixed, or why it was skipped) and what to check:

   ```text
   Where

   Branch acme-shop-5, head 188e2297b82e50d6884e956984042d34d4f88d78 (2 commits on top of 05856b0, not pushed).
   …
   Tests

   go test ./...: ok acme-shop/price (12 cases, all pass).

   Review

   Standards: no findings. Spec: one gap, a missing zero-amount case, fixed in 188e229.
   …
   ```

   Review it and the branch:

   ```console
   $ git log --oneline main..acme-shop-5
   188e229 Make pence conversion explicit in Price.String
   6a8a02e Add Price type that prints its Display price
   $ git diff --stat main acme-shop-5
    GLOSSARY.md         |  7 +++++++
    go.mod              |  3 +++
    price/price.go      | 30 ++++++++++++++++++++++++++++++
    price/price_test.go | 40 ++++++++++++++++++++++++++++++++++++++++
    4 files changed, 80 insertions(+)
   ```

   Then click **Accept**. This run uses the default Landing Method, Merge, so you don't merge or land: at once, the Factory merges the branch into `main`, lands the Ticket, removes its worktree, and claims `acme-shop-6`, which was waiting on it:

   ```text
   acme-shop-5: landed on main at f90edae47659
   acme-shop-5: Done: removed its worktree /Users/you/src/acme-shop.worktrees/acme-shop-5
   acme-shop-6 claimed (implementation): "Negative Prices print with a leading minus, down to math.MinInt64" in Spec acme-shop-1; worktree /Users/you/src/acme-shop.worktrees/acme-shop-6; resume: claude --resume 5b1a7ae5-ea46-48d3-a824-d25a4b4f3b63
   ```

   Review and **Accept** `acme-shop-6` the same way. It lands too:

   ```console
   $ git log --oneline --graph main
   *   210772a Merge acme-shop-6: Negative Prices print with a leading minus, down to math.MinInt64
   |\
   | * d5ebe80 Name the absolute value absPence in Price.String
   | * 5e82a8e Print negative Prices with a leading minus, down to math.MinInt64
   |/
   *   f90edae Merge acme-shop-5: Price prints its Display price for zero and positive amounts
   |\
   | * 188e229 Make pence conversion explicit in Price.String
   | * 6a8a02e Add Price type that prints its Display price
   |/
   * 05856b0 Set up acme-shop as a Code Central Project
   ```

   When it can't land, the Factory leaves the work for you, and says so on the Ticket and in its terminal:

   - If the branch conflicts with the base, it sends the Ticket back to Ready with a comment naming the conflicting files, and its next Claim merges the base in first. The Factory itself checks, at that Claim, whether the Ticket's existing branch still conflicts with the base's tip; if it does, the session's first prompt is not `/implement` but the instructions to merge the base in and resolve the conflicting files, which it names, and its system prompt leaves out the implementation Stage Guide for a short instruction to `finish` with the branch and its head commit. A Ticket sent back for any other reason, or whose branch no longer conflicts, starts as an ordinary `/implement` Claim.
   - If it can't move the base, say because uncommitted changes in `~/src/acme-shop` are in the way, the Ticket stays Accepted with a comment saying why, and the Factory tries again on every poll.

   You can still merge an In Review Ticket's branch yourself: the Factory notices it and lands the Ticket.

   Check: both Tickets are Done, Build shows *2/2*, and the Requirement's row reads *Finished · 2 of 2 Tickets Done*.

8. **Stop the Factory.** Press Ctrl-C in its terminal. With no turns running, it drains and exits at once:

   ```text
   ^C
   draining: no more claiming, resuming or landing; Ctrl-C again to stop the running turns (Claims are kept)
   drained: no turns running
   ```

   With turns running, it prints `draining: waiting on …` and exits once they finish, or stops them at a second Ctrl-C, as in step 1.

   If the answer to a claim was lost on the network, the Ticket may be In Progress already, so the draining Factory still claims it again at each poll until Code Central answers, printing `draining: claiming acme-2 again at each poll first: the answer was lost`, and waits on its turn like the others. A second Ctrl-C gives up on it, leaving any Claim as it is.

   Check: the Factory has exited, and the Requirement is still *Finished*.

## The Factory Status dot

While `code-central-factory` runs, it sends Code Central a Heartbeat on every poll, and the Project's row in the left pane shows a dot before its name: green for Online, amber for Draining and grey for Offline. Hover or focus the dot to list each Factory as name on host, its status, its last Heartbeat and its note, and whether it has Chat support. Project Options has the same list under **Factories**. Archived Projects show no dot.

The notes code-central-factory sends:

| When | State | Note |
| --- | --- | --- |
| each poll | online | `1 of 3 turns running` |
| paused for an outage | online | `paused: <reason>; next resume at 14:30` |
| the first Ctrl-C, and each poll while it drains | draining | `draining: waiting on 2 running turn(s)` |
| it exits after draining | stopping | `stopped: drained` |
| a second Ctrl-C, SIGTERM or SIGHUP | stopping | `stopped on interrupt` |
| a stop only you can fix, such as claude's login expiring | stopping | `stopped: <the reason it prints>` |

A stopping Heartbeat shows Offline at once. A crash sends nothing, so the dot goes grey at the Factory's deadline: three polls, and at least 90 seconds. After Code Central restarts every dot is grey until the Factory's next poll. See [Writing a Factory](writing-a-factory.md) for sending Heartbeats from your own.

### Plan Limits

The Factory also reports the Claude account's Plan Limits, the 5-hour and weekly limits, each as the percentage used and when it resets. It reads them from the `rate_limit_event` claude prints during its Turns, Claim and Chat Turns alike, and every Heartbeat it sends (online, draining and stopping) carries the latest as `plan_limits`. It has none until its first Turn after starting, and it sends no Heartbeat early for new figures: they go with the next poll's. Only a Turn brings new figures, so they grow stale while the Factory is idle; it keeps the last ones until it exits. The limits are account-wide, so with several Factories the Project shows the most recently read.

### Chat in the UI

While a Project is open (not in the all-Projects scope) and one of its Factories is Online with Chat support, a round Chat icon floats in the lower right corner. It opens a pop-up holding the Project's current Conversation, the same in every tab. Your messages show as plain text and Claude's as Markdown, streaming in as it writes. Each tool call is one collapsed line that opens to its result, a denied permission shows as a warning, and a stopped or failed reply ends with `Stopped` or its error. **Send** is off while a reply runs, and **Stop** shows then. Sending with no Conversation starts one with the Chat row's model and effort. **New conversation** asks for a model and effort first, filled from that row (haiku when it sets none), and drops the old Conversation. **Copy resume command** copies `cd <worktree> && claude --resume <session-id>` once the session is known.

If the Conversation's Factory is no longer Online, or no Factory has Chat support, the open pop-up stays and says so, with Send off. It becomes usable again when the Factory is back Online, or after New conversation. When a long Conversation drops its oldest messages, the pop-up says `Older messages were dropped`.

### The Chat worktree and Chat turns

Each Conversation turn runs `claude -p` in the **Chat worktree**, `chat` in the Factory's worktrees folder beside the Tickets' (`~/src/acme-shop.worktrees/chat`); a Ticket ID is never `chat`, so the two can't clash. A Conversation's first turn moves the worktree to the base's tip, uses `--session-id` and prints `chat: starting a Conversation in <worktree>; resume: <command>`; later turns use `--resume`. The Factory keeps one `next_chat_work` call open and runs the turns it hands out one at a time.

With several Repositories, each has its own Chat worktree beside its clone, `<clone parent>/<Repository Name>.worktrees/chat`, which a Conversation's first turn moves to that Repository's Base tip. The turn runs in the Home Repository's Chat worktree and reaches the others with `--add-dir`, and the Conversation's Resume Command carries them. The sweep leaves every Chat worktree alone.

A Chat turn takes **no Claim slot**: it doesn't count against the Factory's concurrent turns, and a Conversation never waits for a Claim to end.

- **Draining.** The first Ctrl-C closes the `next_chat_work` call, so no new Chat turn starts. A turn already running goes on, and the Factory exits once it has ended. While its Factory is Draining, the pop-up shows the Conversation as unavailable.
- **Stopping.** A second Ctrl-C, SIGTERM or SIGHUP stops a running Chat turn as it stops a Claim's, and the reply ends `Stopped`. **Stop** in the pop-up stops just that reply: the Factory hears it on its `next_chat_work` call.
- **An Outage or an expired login.** A reply cut off by an Outage (a usage or rate limit, overload, or a server or network error) or by an expired or missing login fails with text naming the cause: "claude hit an Outage (`rate_limit`): a usage or rate limit, overload, or a server or network error cut off the reply. Send the message again later." or "claude's login has expired or is missing (authentication_failed): log in with `claude /login` on the Factory's host, then send the message again." Chat doesn't pause or stop the Factory for either, and files no Failure Report.
- **`--no-chat`.** The Factory opens no `next_chat_work` call and reports `chat: false`, so Chat isn't available through it.

## Model and effort

Before every turn, a first turn or a resume after answers, after a cut-off or after a restart, the Factory works out the model and effort the turn runs with. Each is worked out on its own, highest first:

1. the Ticket's `model` or `effort` Tag;
2. the Project's Models row for the Ticket's kind of work, its Stage Guide name;
3. the Project's default Models row;
4. the Factory's `--model` or `--effort` flag;
5. Claude Code's own default: the Factory passes no flag.

Set tiers 2 and 3 in the [Models section of Project Options](setting-up-a-project.md#choosing-models-for-each-kind-of-work): a Project default row, and a row for each kind of work. A Ticket's kind of work comes from its Labels: `grilling`, `spec` or `tickets` for a Stage, `map-chart`, `map-grilling`, `map-research` or `map-task` for a Map Ticket, `prototype` for a Prototype, `retro` for a Retro, and `implementation` for any other Ticket. Override them for one Ticket with its Tags, from the Ticket panel, from an agent with `set_tags`, or by hand:

```sh
code-central set-tags acme-shop-5 model=opus effort=high
```

The Ticket panel shows the model and effort the Ticket's next turn runs with, each with where it comes from: `opus (Ticket Tag)`, `opus (the Project's implementation row)`, `opus (Project default)`, `sonnet (Factory flag)` for the flag the Factory reported it was started with, or `Claude Code's default`. Below each is its picker. The **Model Tag** suggests the aliases and model IDs the Factory reported and takes any other name: **Set** writes the `model` Tag and **Clear** removes it. The **Effort Tag** offers the valid levels: picking one writes the `effort` Tag, and picking **Project's settings** removes it. Without its Tags the Ticket goes back to its Project's settings. The panel follows the Ticket's Tags, the Models rows and the Factory's reports as they change.

`effort` must be one of the valid levels: `low`, `medium`, `high`, `xhigh` and `max` until a Factory has reported its `claude`'s, below, and then those. `set_tags` rejects anything else and lists them. `model` takes any name `claude --model` takes. On a Spec, `model` and `effort` are ordinary Tags with no effect.

### What `claude` accepts

When it starts, the Factory runs `claude --help` and reads the effort levels from the `--effort` option's list, `(low, medium, high, xhigh, max)` in Claude Code 2.1.311, and the model aliases from the names quoted in the `--model` option's text, such as `'fable'`, `'opus'` and `'sonnet'`. It reports them to Code Central with its own `--model` and `--effort` flags. Code Central keeps them as the Project's Reported Models: from then on the valid effort levels are the reported ones, and the [Models section](setting-up-a-project.md#choosing-models-for-each-kind-of-work) offers them. A row or Tag already set to a level a later report drops is kept as it is. If the Project can't be read when the Factory starts, it reports on the first poll that can read it. As each session's usage arrives, Code Central also adds to the Reported Models any model ID it names that they don't have yet, such as `claude-opus-5-5`; model IDs are never removed.

An `--effort` that isn't one of the levels `claude --help` lists stops the Factory at the start:

```console
$ code-central-factory --effort extreme
code-central-factory: refusing to start: --effort "extreme" isn't an effort level claude accepts: use one of low, medium, high, xhigh, max
```

If it can't read the levels or the aliases, say from a `claude` whose help reads differently, the Factory prints a warning and carries on, and `--effort` isn't checked:

```console
$ code-central-factory
warning: can't read the effort levels from claude --help, so none are reported, and --effort isn't checked
warning: can't read the model aliases from claude --help, so none are reported
base: main
```

What it can't read is reported as nothing, which leaves the levels and aliases an earlier Factory reported as they were; the flags are always reported.

The choice is worked out afresh each turn, so a Tag or Models row changed while a Ticket waits on a Round applies when it resumes, with no Factory restart. Each Claim in the Ticket's Claims records the model and effort its latest turn ran with, or Claude Code's default. Only the sessions the Factory starts are affected: subagents they start, such as code-review's reviewers, keep their own agent definitions' models.

### A model or effort `claude` won't take

A bad model or effort never quietly wastes a Claim or runs at the wrong effort: either one fails the Claim, and the Ticket goes to Failed with a Failure Report that names the value, where it was set and what to change. Where it was set is one of the Ticket's `model` or `effort` Tag, the Project's Models row for its kind of work (named), the Project's default Models row, or the Factory's `--model` flag.

`claude` ignores an `--effort` it doesn't accept and runs at its default, so before every turn the Factory checks the turn's effort against the levels it read from `claude --help`. An effort that fails starts no turn. This catches a Tag or Models row set while other levels were valid, which a later report dropped. When the Factory couldn't read the levels, it checks nothing. Say the Ticket's `effort` Tag is `xhigh` and this `claude` lists `(low, medium, high)`:

```console
acme-shop-5: Failed: The effort `xhigh`, set by the Ticket's `effort` Tag, isn't one this Factory's claude accepts (low, medium, high), so no turn was started.
```

The Failure Report's *What to fix* reads: Set the Ticket's `effort` Tag to one that claude accepts (low, medium, high), or remove it (in the Ticket panel, or with `set_tags` or `remove_tags`), then send the Ticket back.

A model `claude` rejects ends the turn with the API error `model_not_found`. Unlike an outage it doesn't pause the Factory, and unlike an expired login it doesn't stop it: the Claim fails and the Factory carries on. Say the Project's Models row for implementation sets `opus-9`:

```console
acme-shop-5: Failed: claude rejected the model `opus-9`, set by the Project's Models row for implementation (model_not_found).
```

Its *What to fix* reads: In Project Options, set the model in the Models row for implementation to one that claude accepts, or to Default, then send the Ticket back. Its *Why* quotes what `claude` said.

## Claude Args

Factory sessions can't turn on the Chrome extension with an interactive `/chrome`, and Claude Code only enables it with `--chrome`. Everything after `--` on the Factory's command line is **Claude Args**: `claude` flags the Factory passes, as given, to every start and resume of every Claim, so agents can use the browser to see a UI change in the running app:

```console
$ code-central-factory --parallel 1 -- --chrome --add-dir ../shared
base: main
passing to every claude turn: --chrome --add-dir ../shared
```

Any `claude` flag works the same way, written as you would type it to `claude`. A resumed session gets them again, and after a restart it gets the restarted Factory's. They stay on the machine: Code Central never sees them. Parallel sessions share one Chrome, so if browser work collides, try `--parallel 1`.

The Factory refuses to start when a Claude Arg is a flag it sets itself, in its `--flag=value` form too, and points at its own flag where it has one:

```console
$ code-central-factory -- --model=opus
code-central-factory: --model=opus can't be a Claude Arg: use the Factory's --model
```

Those are `--model` and `--effort` (use the Factory's), `--permission-mode`, `--permission-prompts` and `--dangerously-skip-permissions` (use `--bypass-permissions`), and the ones the Factory sets itself: `-p`/`--print`, `--session-id`, `-r`/`--resume`, `-c`/`--continue`, `--output-format`, `--input-format`, `--allowedTools`/`--allowed-tools`, `--append-system-prompt`, `--system-prompt`, `--system-prompt-file`, `--no-session-persistence` and `--bare`. A bare `--` among them is refused too.

With Claude Args, the Factory also checks at startup that `claude` accepts them, by running `claude -p` with them once, with no prompt and empty input. That makes no model call and leaves no session behind. A typo stops the Factory:

```console
$ code-central-factory -- --chrom
code-central-factory: refusing to start: claude doesn't accept the Claude Args: error: unknown option '--chrom'
```

If `claude` answers in a way the Factory doesn't recognise, it prints a warning with the output and carries on. The check doesn't prove Chrome is connected; `--chrome` only makes the browser tools available. When a turn exits with no result message, its Failure Report's *What to fix* also lists the Claude Args the turn started with, so you suspect them if `claude` rejected one.

## Map Tickets and Prototypes

A Map's Tickets each carry a Wayfinder Label, and the Factory claims them like a Stage's Ticket, by the skill its Label names. A Prototype and a Retro are claimed by their skills too:

| Label | Guide | Skill |
| --- | --- | --- |
| `wayfinder:chart` | `map-chart` | `/wayfinder` |
| `wayfinder:grilling` | `map-grilling` | `/grilling`, with `/domain-modeling` alongside |
| `wayfinder:research` | `map-research` | `/research` |
| `wayfinder:task` | `map-task` | none: the session starts from the Guide alone |
| `prototype` | `prototype` | `/prototype` |
| `retro` | `retro` | `/retro` |

A Map's Charting, grilling and task Tickets and a Retro each run in a read-only worktree detached at the base's tip, as a Stage does, and write to Code Central, not to the repository. A Retro's system prompt also says where Claude Code keeps the logs of the sessions it reads. Each one's line names its Guide in place of the Stage:

```text
acme-shop-9 claimed (map-chart): "Charting: Dark mode" in Spec acme-shop-8; worktree /Users/you/src/acme-shop.worktrees/acme-shop-9; resume: claude --resume …
```

A **Prototype** is a Ticket labelled `prototype`, on any Spec. Ask for one with **New Prototype** under the title of the Spec's view in the UI (Requirement, Map or plain Spec), or submit one with `code-central submit-prototype <spec> --text "…"` (or `submit_prototype` over MCP); its first line becomes the title, after "Prototype: ". The Factory claims it with `/prototype` and runs it in a worktree of its own, as for implementation, but on a local branch `prototype/<ticket>`:

```text
acme-shop-10 claimed (prototype): "Prototype: A theme picker in the header" in Spec acme-shop-8; worktree /Users/you/src/acme-shop.worktrees/acme-shop-10; resume: claude --resume …
```

Its system prompt tells the session to commit on that branch only, with no push and no PR, to record where the Prototype is in a `prototype` Tag (`prototype/acme-shop-10@1a2b3c4 /Users/you/src/acme-shop.worktrees/acme-shop-10`), to open its first Round with a "Try it" Question, and to finish with no branch. The Ticket panel shows the Tag as *Prototype at `/Users/you/src/acme-shop.worktrees/acme-shop-10` on `prototype/acme-shop-10` (`1a2b3c4`)*, so you can try it in that worktree while you answer.

Because it finishes with no branch, accepting a Prototype makes it Done straight away, whatever the Landing Method: nothing is merged, pushed or landed. The worktree stays through Needs Answer and In Review, and the Factory removes it once the Prototype is Done or Cancelled. It never deletes the `prototype/` branch, so `git switch prototype/acme-shop-10` brings the Prototype back.

A Prototype can also come from a Round. Answer a Question from the grilling Stage, Charting or a Map's grilling Ticket with **Prototype this**, a button on each Question that fills the answer with `Prototype this: ` for your note. The session creates a Ready Prototype on the same Spec from the Question and your note, makes it block the asking Ticket, and gives up. The Factory claims the Prototype as above. Once you accept it, the asking Ticket is unblocked, and the Factory claims it again as a redo, which carries on from the Prototype's resolution comment.

A Map's **research** Ticket (`wayfinder:research`) runs in the background with `/research`, in a worktree of its own on a local branch `research/<ticket>`, just as a Prototype does:

```text
acme-shop-11 claimed (map-research): "Which colour libraries exist?" in Spec acme-shop-8; worktree /Users/you/src/acme-shop.worktrees/acme-shop-11; resume: claude --resume …
```

Its system prompt tells the session to commit its findings on that branch only, with no push and no PR, to record where they are in a `research` Tag (`research/acme-shop-11@9f8e7d6`), and to finish with no branch. Accepting it makes it Done straight away, whatever the Landing Method, with nothing merged, pushed or landed. The Factory removes its worktree once it is Done or Cancelled and never deletes the `research/` branch, so `git show research/acme-shop-11` still finds the findings.

A **Retro** is a Ticket labelled `retro` on the Project's Retros Spec, a Long-lived Spec Code Central creates on first use. Ask for one on a finished Ticket (Done or Cancelled) or Spec with `code-central submit-retro <ticket|spec> [--note "…"]` (or `submit_retro` over MCP); it opens Ready, titled "Retro: " and the target's title, and its description lists every Claim on the work to read. The Factory claims it with `/retro`, as the table shows. It looks back over those sessions and proposes changes to the agents' environment, and its work never lands:

```console
$ code-central submit-retro acme-shop-4 --note "Why did this take three redos?"
{
  "id": "acme-shop-12",
  "state": "Ready"
…
}
```

A Ticket with any of these Labels is never run as an implementation Ticket.

Selecting the Map in the **Requirements** view opens the Map view: **New Prototype** and its Destination, then its Tickets as a **Ticket graph** read left to right, each Blocker before what it blocks, coloured like the Stage strip, and a dim **Not yet specified** node at the right edge that shows that section when clicked. A Draft Ticket that nothing blocks is on the frontier: click **Start** on its node to approve it to Ready, and the Factory claims it at once. Clicking a node opens its Ticket below. **Decisions so far** and **Out of scope** sit beside the graph.

To let the Factory find the way for you, tick **Guided** in the Map view's header. It sets the Map's `guided` Tag to `on`, and from then on, on each poll, the Factory checks whether the Map is idle: every Ticket on it that isn't research is Draft, Done or Cancelled. When it is, the Factory approves the Map's first frontier Ticket in created order, one per poll, and claims it as usual:

```text
acme-shop-12 approved (guided Map acme-shop-8): "Which store?"
acme-shop-12 claimed (map-grilling): "Which store?" in Spec acme-shop-8; worktree /Users/you/src/acme-shop.worktrees/acme-shop-12; resume: claude --resume …
```

Any Ticket on the Map that is Ready, In Progress, Needs Answer, Answered, Failed, In Review or Accepted holds it, so you are never asked two things about one Map at once; a research Ticket running in the background doesn't. The Factory starts nothing while it is paused for an outage or draining. Untick **Guided** to remove the Tag and hand the Map back to your **Start** buttons.

Once every Ticket on the Map is Done or Cancelled, the Map view's header has **Write the Spec**. It opens the **New requirement** dialog with the Map's title, a link to the Map and its Destination already filled in. Edit the text if you like, then click **Submit requirement**. Code Central creates a new Requirement with only the spec and tickets Stages, and its spec Stage is Ready straight away. Its spec Stage's review has the same choice, **Split into Tickets** or **Implement**. The Factory claims it at once like any spec Stage, and the agent works from the Map's Decisions so far and the resolution comments they link, in place of a grilling Ticket. The Requirement's strip shows a done **Map** segment where Grilling would be, linking back to the Map. The Map stays as the record, and its header links each Requirement written from it. From a terminal, `code-central submit-requirement --map <map> --text "…"` does the same.

When a Map's Ticket asks you a Round or finishes into In Review, it waits on you in **Needs Me** and Focus Mode as its own entry, not folded into a row as a Stage Ticket is. Its row reads `↳ Map: Dark mode` under its title, and its Focus Item reads *Map: Dark mode*. A Prototype also carries a **Prototype** badge, and on a plain Spec its row names that Spec instead.

## How accepted work lands

Each Project has a **Landing Method**, its **Default Landing**, which you set on its **Project Options** page, from the three-dots menu on the Project. Each Repository follows it or has its **Own Landing**, and lands by that (see **Mixed Landings** below). The Factory reads them again on every poll, before it lands anything, so a change applies from its next poll to every Ticket still waiting to land, with no restart.

- **Merge**, the default, is what step 7 shows. The Factory merges an accepted Ticket's branch into the base and lands it. When the branch conflicts with the base it sends the Ticket back to Ready; when it can't move the base it leaves the Ticket Accepted with a comment and tries again on every poll. It keeps an Accepted Ticket's worktree until the Ticket lands, since a Ticket sent back is worked in it again.
- **Branch** ("Leave as a branch") leaves the merging to you. The Factory never merges an Accepted Ticket and never sends it back for conflicts: a conflict in a branch you merge is yours to resolve. At once on your **Accept**, it says so once and removes the Ticket's worktree, keeping its branch, so you can check the branch out:

  ```text
  acme-shop-5: Accepted under the Branch Landing Method: merge acme-shop-5 into main and the Factory lands it
  acme-shop-5: Accepted: removed its worktree /Users/you/src/acme-shop.worktrees/acme-shop-5
  ```

  A worktree with uncommitted changes is left where it is, with a warning line. The Ticket stays Accepted, so a Ticket it blocks isn't claimed yet. Merge the branch into the base yourself, and on its next poll the Factory sees the Ticket's finish commit (or, with none, its branch's tip) in the base and lands it at that commit:

  ```console
  $ git merge --no-ff acme-shop-5
  ```

  ```text
  acme-shop-5: merged into main by hand
  acme-shop-5: landed on main at 188e2297b82e
  ```

  The Factory recognises a merge only when the commit itself is in the base. After a squash or rebase merge it isn't, so land the Ticket yourself: **Land** on its panel, with the commit that put the work into the base.

- **Pull Request** hands the merge to the Code Host, GitHub or Azure Repos. Accepting pushes the Ticket's branch, a Pull Request Watcher opens a pull request for it, and the Ticket lands when the host reports it merged. See [Pull Request](#pull-request) below.

Switching from Branch to Merge merges, on the next poll, every Accepted Ticket you haven't merged yet. Switching from Merge to Branch leaves every Accepted Ticket the Factory hasn't merged yet for you.

Under either method, you can still merge an In Review Ticket's branch yourself, and the Factory lands it.

A Spec's `branch` Tag renames only its Spec Branch, so it has no effect under Branch: see [Land per Spec](#land-per-spec).

### Land per Spec

Under Merge with Land per Spec on, the accepted work of every Spec but a Long-lived one first gathers on its Spec Branch, `spec/<spec id>`. A Long-lived Spec is one labelled `long-lived`, as a CI Rule's Spec and the Issues Spec are; its Tickets land one by one. Once every implementation Ticket on the Spec is Accepted, Done or Cancelled (and, for a Requirement, its tickets Stage is finished), and at least one is merged into the Spec Branch, the Factory merges the Spec Branch into the base as `Merge spec <spec id>: <title>`, and lands every waiting Ticket at that commit. With Push on, it pushes the base once and lands them after the push succeeds; with CI Tracking on, the same pushed commit is reported for each, they all land once its CI is passed, and if it fails they are listed on the commit's one CI failed item, with no Send back to fix, since their work is in the base with the Spec's. Only the base is pushed.

```text
acme-shop-5: merged into spec/acme-shop-2 at 3b9e1f0c2d44
acme-shop-2: merged spec/acme-shop-2 into main at 9ad0fd655dd2
acme-shop-2: landing its Tickets: acme-shop-5, acme-shop-6
acme-shop-5: landed on main at 9ad0fd655dd2
acme-shop-6: landed on main at 9ad0fd655dd2
```

When the Spec Branch conflicts with the base, the Factory asks Code Central to open a Spec merge Ticket: a Ready Ticket on the Spec, "Merge the base into the Spec Branch of <spec id>" (under [Rebase](#rebase), "Rebase the Spec Branch of <spec id> onto the base"), naming the conflicting files and both branches' tips, which Code Central marks as a Spec merge Ticket. While that Ticket is open, a later conflict is commented on it instead, and the same conflict again writes nothing. The Factory retries once the Ticket is accepted. A Ticket with that title made any other way, or opened by a Factory from before the mark, is not marked; Code Central comments on it the same way but leaves it unmarked.

```text
acme-shop-2: spec/acme-shop-2 conflicts with main in README.md: opened acme-shop-7 to merge the base into it
acme-shop-2: spec/acme-shop-2 conflicts with main again: commented on acme-shop-7
```

**Naming the Spec Branch.** A Spec's `branch` Tag, a reserved Spec Tag beside `map` and `effort`, names its Spec Branch in place of `spec/<spec id>`. Set it with `set_tags` or the Tags editor on the Spec pane, say `branch` = `feat/1234`, and Ticket branches are cut from, merged into and pushed as `feat/1234`, and the Factory's log lines say so. A Requirement made from an Azure Boards work item gets it from the Project's Branch format. The value must be a valid branch name, not `spec/…`, a Repository's Base or a Ticket ID, and no other unfinished Spec may use it; the refusal names the Spec that does. The Tag is **locked** once any implementation Ticket of the Spec has been claimed, as the Spec Branch exists by then: changing or removing it is refused, saying the Spec Branch already exists under that name, and setting the same value is fine.

If the Push remote already has a branch of that name that the Factory didn't push (none of the Spec's Tickets has a pushed commit on it), the Factory won't push over it, forced or not, even under Rebase. It comments once on each waiting Ticket and tries again on every poll, so once you delete or rename the remote branch the push goes ahead. This holds for `spec/<spec id>` names too:

```text
origin already has a branch named feat/1234 that the Factory didn't push, so it won't push the Spec Branch over it. Delete or rename it on origin, and the Factory pushes the Spec Branch on a later poll.
```

The Tag names only the Spec Branch. With Land per Spec off, under the Branch Landing Method, and on a Long-lived Spec it has no effect, and Ticket branches keep their Ticket-ID names. A finished Spec's Tag-named Spec Branch is deleted by the rule below, as a `spec/` one is.

Turning Land per Spec off part-way makes the Factory's next poll merge every Spec Branch that holds merged-but-not-landed Tickets into the base, ready or not, and land them as a group; later Tickets land one by one. Adding the `long-lived` Label to a Spec while its Tickets wait does the same for that Spec alone. Turning Land per Spec on, or taking the Label off, affects only Tickets accepted afterwards. A Ticket added to a Spec after it has landed lands with a second batch: the Spec Branch is merged into the base again once the new Ticket is accepted.

A Spec Branch is deleted once it is in the base and every Ticket on its Spec is Done or Cancelled. One that isn't in the base, as when the whole Spec is cancelled, is kept, and the Factory says so once. With several Repositories, each one's Spec Branch goes by the same rule against its own Base. Ticket branches are never deleted, but with several Repositories a branch the Ticket never used is (see **Worktrees in every Repository** below).

```text
acme-shop-2: deleted spec/acme-shop-2, which is in main
acme-shop-4: every Ticket is finished but spec/acme-shop-4 isn't in main: kept
```

### Rebase

With **Rebase instead of merging** ticked (under Merge or Pull Request; see [Setting up a project](setting-up-a-project.md#choosing-how-accepted-work-lands)), the Factory replays a Ticket's commits instead of merging its branch, so history stays linear with one merge commit per landing:

- Under Land per Spec, a Ticket accepted onto its Spec Branch is replayed onto it, and before the one `Merge spec <spec id>` commit the Factory rebases the Spec Branch onto the base. When that conflicts, the Spec merge Ticket is "Rebase the Spec Branch of <spec id> onto the base", and accepting it replaces the Spec Branch.
- With Land per Spec off, each Ticket's branch is replayed onto the base before its `Merge <id>` commit.
- Under Pull Request, a branch about to be pushed is rebased onto the fetched base first and pushed with `--force-with-lease`. The Factory force-pushes only its own Ticket and Spec branches, never a base or a branch of the Spec Branch's name it didn't push (see [Pull Request](#pull-request)), and when a rebase conflicts it pushes in no Repository.
- A conflict Claim is asked to rebase the branch onto the base, not merge the base into it.

```text
acme-shop-5: replayed onto spec/acme-shop-2 at 3b9e1f0c2d44
acme-shop-6: replayed onto main at 7c41a9e0d2b8
```

Leave as a branch is unaffected.

### Pull Request

Under **Pull Request**, a Repository's Push remote must be on GitHub or Azure Repos, and Push is implied: the Factory pushes each Ticket's branch to the remote once you accept it, and reports the pushed commit and branch to Code Central. The Pull Request Watcher (see [The Pull Request Watcher](running-as-a-service.md#the-pull-request-watcher)) opens a ready-for-review pull request into the base from that branch, with the Ticket's title and finish summary, and reports its state. You merge it on the host, by any of its merge buttons. The Factory never calls the host and never merges the Ticket itself:

```text
acme-shop-5: pushed acme-shop-5 to origin
```

Before it lands anything or cuts a branch, the Factory fetches the remote and fast-forwards the local base to the remote's. It never merges into, rebases or force-pushes the base. This is the one method under which the Factory pulls.

```text
main: fast-forwarded to origin/main at 22eeda3a9de2
```

When the host reports a Ticket's pull request merged, the Ticket lands at the commit the host merged it at, however it merged, squash and rebase included, once the fetched base holds that commit; until then the Factory says it is waiting:

```text
acme-shop-5: pull request 7 merged
acme-shop-5: landed on main at 22eeda3a9de2
```

```text
acme-shop-5: its pull request is merged at 22eeda3a9de2, which main doesn't hold yet: waiting for it
```

A Ticket whose branch has nothing the base lacks lands at once, with no pull request. A failed push leaves the Ticket Accepted with a comment, tried again on every poll; the Factory also pushes any Accepted Ticket with no pushed commit yet, such as one accepted before you switched to Pull Request. A failed fetch is a warning and a retry:

```text
warning: can't fetch origin, so nothing lands this poll: fatal: unable to access 'https://example.com/acme-shop.git/': Could not resolve host: example.com
```

**Base diverged.** When the local base has commits the remote's lacks, it can't be fast-forwarded (under [Merge + Push](#push), where a base that is merely ahead is current, it is diverged only when the Factory couldn't move it onto the remote's). The Factory reports it, then lands nothing and claims no work that would cut a branch, until you reconcile the local base with the remote's, for example by resetting or rebasing it. Its next poll sees the fix by itself. Code Central shows a **Base diverged** item in Needs Me and Focus Mode, with both commits:

```text
warning: the base, main, has diverged from the remote's: it is at 9ad0fd655dd2 and the remote's at 22eeda3a9de2. Nothing lands and no branch is cut until you reconcile them by hand
main: no longer diverged from the remote's
```

**Base missing.** Under every Landing Method, a base that exists neither locally nor on the Push remote is reported as missing through `report_base`. Code Central shows one **Base missing** item per Project in Needs Me and Focus Mode, naming the branch and the fix: create the branch, or change the Base branch in Project Options. The report clears when a later poll reports the base current. With [several Repositories](setting-up-a-project.md#several-repositories), each Repository's base is reported apart, and Base missing and Base diverged are one item per Repository, naming it (see **Landing across Repositories** below).

On every poll, before it claims or lands, the Factory looks for the base. A local branch is used as it is. A base that exists only on the Push remote is created locally at the remote's commit, after a fetch; a local-only Repository's remotes aren't the Repository's, so it is never fetched and its base is its local branch or missing. If it exists in neither place the Factory keeps running and heartbeating, but claims nothing and lands nothing until you create the branch or change the setting. It needs no restart:

```text
base: created release at 9ad0fd655dd2 from origin/release
warning: the base, release, exists neither here nor on origin: nothing lands and no branch is cut until you create it or change the Base branch in Project Options
base: found release again
```

**Repository missing.** In a Project with [several Repositories](setting-up-a-project.md#several-repositories), the Factory also makes sure, on every poll, that each one has a clone: at its local path, else beside the Home Repository, at `<parent of the Home Repository>/<Repository Name>`. It clones a Repository with nothing at that path under the Push remote's name (`git clone --origin origin`), and adopts a clone already there when its Push remote's URL is the Clone URL (ssh or https, with or without `.git`). A Repository added while the Factory runs is cloned on its next poll. A **local-only** Repository, one with no Clone URL, is never cloned: the Factory adopts the git repository at its local path whatever its remotes, and never adds or rewrites one. Anything else at the path, a clone that fails, nothing at a local-only Repository's path, or a path that an earlier Repository in the landing order or the Home Repository already has, is **Repository missing**: the Factory touches nothing there, reports it through `report_repositories`, and starts no new Claim until every Repository has a clone. Code Central shows one **Repository missing** item per Repository in Needs Me and Focus Mode, naming the path, the Clone URL, the problem and the fix: clone it there by hand, set its local path in Project Options to a clone you have, or move what's there aside for the Factory to clone it; for a local-only Repository, put its git repository at the path or change its local path. Project Options shows each Repository's clone and path. The next poll sees the fix by itself:

```text
repository web: cloned git@github.com:acme/web.git to /home/you/src/web
repository api: adopted the clone at /home/you/src/api
warning: Repository docs is missing: /home/you/src/docs isn't a git repository: no new Claim starts until it has a clone
warning: Repository notes is missing: nothing at /home/you/src/notes: no new Claim starts until it has a clone
warning: Repository tools is missing: /home/you/src/api is Repository api's clone: no new Claim starts until it has a clone
repository docs: cloned git@github.com:acme/docs.git to /home/you/src/docs
```

**Worktrees in every Repository.** With several Repositories, each Claim's start makes or reuses the Ticket's worktree in every one, beside its clone: the **Home worktree** at `<home>.worktrees/<ticket-id>` as ever, and each other at `<clone parent>/<Repository Name>.worktrees/<ticket-id>`. The branch has the same name in each (`<ticket-id>`, `prototype/<ticket-id>` or `research/<ticket-id>`), cut from that Repository's own Base, or under Land per Spec from its own Spec Branch, made from its Base on first use. A Stage gets a worktree detached at each Repository's Base tip, and a dirty one fails the Claim. A failure setting up a worktree names its Repository, and a Ticket whose branch conflicts with the Base in any Repository starts as a Conflict Claim listing the files under each Repository's name. The session starts in the Home worktree and reaches the others with `--add-dir`; its first prompt ends with a Repositories list giving each one's worktree, Base and branch, and its system prompt says to commit in each worktree and finish with `repositories`. The Ticket's Resume Command carries the same `--add-dir` flags. A Repository added while a Claim is held gets its worktree at the Ticket's next Claim; resuming the session doesn't add it:

```text
acme-shop-14 claimed (implementation): "Show prices from the API" in Spec acme-shop-13; worktree /home/you/src/acme-shop.worktrees/acme-shop-14; resume: claude --resume 2c9e4b1a-6f0d-4e8a-9b57-3d1f0a6c2e84 --add-dir /home/you/src/api.worktrees/acme-shop-14
```

Once the Ticket is done with its worktrees, the sweep removes them in every Repository, and a branch the Ticket never used (still on the line of the Base or Spec Branch it was cut from) goes with its worktree. A branch with work of its own is kept, merged or not. In the Home Repository, whose branch landing reads, an unused branch goes only once the Ticket is Done or Cancelled, or when it names no Home branch; with the Home Repository alone no branch is ever deleted. Here the Ticket changed the Home Repository alone:

```text
acme-shop-14: Done: removed its worktree /home/you/src/acme-shop.worktrees/acme-shop-14
acme-shop-14: Done: removed its worktree /home/you/src/api.worktrees/acme-shop-14
acme-shop-14: deleted its unused branch acme-shop-14 in Repository api
```

**Changed Repositories and the finish check.** With several Repositories, a Ticket's **Changed Repositories** are those whose branch holds commits in neither their Base nor, under Land per Spec, their Spec Branch; a Stage changes none. On every poll the Factory works them out from the branches of each Ticket it holds In Progress and reports them to Code Central (`report_changed_repositories`) whenever the set or a head moves, so the Ticket's card and page show them as the work goes. The finish settles them: `branch` and `commit` for the Home Repository and `repositories` for each other one, as the session is told (`code-central finish` takes them as `--repositories '[{"name":"api","branch":"acme-shop-14","commit":"4f1c2d9"}]'`). After the turn that finishes a Ticket, and again before landing it, the Factory checks the finish against every Repository's branch. A finish that leaves out a Repository with commits, or records a commit that isn't its branch's head, fails: the Ticket moves to Failed with a Failure Report on the finishing Claim (`fail_finish`), naming what is wrong and where each branch stands, and nothing of it lands. Send it back and its next Claim finishes again. An Accepted Ticket is failed this way only while it has landed nowhere, and a finish that drops a Repository its Ticket had recorded isn't auto-accepted, so the check comes first. With the Home Repository alone, nothing is reported or checked:

```text
acme-shop-14: the finish check failed: api is left out, but its branch acme-shop-14 holds commits beyond main, up to 4f1c2d9e8a7b
acme-shop-14 turn ended (implementation): "Show prices from the API": exit 0, success (completed); now Failed; ran 6m12s
```

**Landing across Repositories.** Under Merge, an accepted Ticket lands in each of its Changed Repositories, and is Done once it has landed in all of them. The Factory first works out the merge in every one it hasn't landed in yet. If any conflicts, it lands none, and sends the Ticket back to Ready with one comment naming each conflicting Repository and its files. Otherwise it goes through them in the **landing order**, the order of Project Options' Repositories list (the Home Repository first by default): in each it makes the `Merge <ticket>: <title>` commit, advances that Repository's base, pushes it with Push on, and lands the Ticket there, recording that Repository's landed commit. A Ticket that has landed in some of its Repositories but not all is **Partly landed**, a badge on its card and page, and can't be cancelled. A failed push or base move stops there: the Factory comments once, naming the Repository, and tries again on every poll. It never force-pushes, resets or reverts what has landed. Under Merge + Push with CI Tracking on, the [CI gate](#with-ci-tracking) holds in each Repository whose CI is watched: once its merge is pushed there, the Ticket lands there when its CI result there passes, at a pushed commit holding the work, so a red build in one Repository holds back only that Repository's landing. The next Repository goes ahead once the push is made, and a **Not watched** Repository lands once pushed. A **local-only** Repository, one with no Clone URL (`scripts` below), is never pushed, fetched or kept level, so Merge + Push reads there as Merge: the Ticket lands there once merged into its base, with no CI gate, no pushed commit for CI to track and no Land anyway, and its base is never Base diverged, only Base missing when it isn't there locally. A Ticket that changed only local-only Repositories lands once merged. A Ticket left Accepted a while is Accepted but not landed, naming the Repositories it has landed in and those it hasn't, each with why: its broken base, its CI there not passed yet, its pull request there still open, or what the Factory last commented, such as a refused push. Each Repository's base is located, kept level with its own remote unless the Repository is local-only, and moved as the Project's is, and a missing or diverged one is a **Base missing** or **Base diverged** item naming it. It holds back only the Tickets landing in it, and no new Claim starts until it is fixed. The `land` operation names the Repository with `--repository` (`repository` over MCP); left out, it means the Home Repository. With the Home Repository alone, nothing of this shows and landing is as it always was:

```text
acme-shop-14: pushed main to origin
acme-shop-14: landed on main at 7d1e0c9a2b3f
acme-shop-14: can't land yet in api: couldn't push main to origin
acme-shop-14: pushed main to origin in Repository api
acme-shop-14: landed on main at 4b5c6d7e8f90 in Repository api
acme-shop-15: waits to land: its base, release, is missing in Repository api
acme-shop-16: pushed main to origin in Repository api
acme-shop-16: CI tracks main at 0a1b2c3d4e5f in Repository api
acme-shop-16: CI failed at 0a1b2c3d4e5f in Repository api, so it stays Accepted
acme-shop-17: landed on main at 5f6a7b8c9d0e in Repository scripts
warning: the base, release, doesn't exist in Repository scripts, which is local-only: nothing touching it lands and no branch is cut until you create it or change its Base in Project Options
```

Under **Branch**, an accepted Ticket lands in each of its Changed Repositories once you have merged its branch there into that Repository's base, and is Done once you have merged it in all of them; merged in some, it is Partly landed. With Push on, accepting pushes its branch in each that has a Clone URL, and in a local-only one pushes nothing. Under **Land per Spec**, each Repository has its own Spec Branch. An accepted Ticket is merged into its Spec Branch in each of its Changed Repositories, every one checked first: a conflict in any merges it into none, and sends it back with one comment naming each conflicting Repository and its files. Once the Spec is ready, the Factory checks the Spec Branch of every Repository the Spec's Tickets changed against that Repository's base before it lands any. A conflict in any lands nothing and opens, or comments on, the Spec's one Spec merge Ticket, which names each conflicting Repository with its files and both tips; its Claim merges each base into its branch there. Otherwise the Factory makes the same `Merge spec <spec id>: <title>` commit in each, in the landing order, pushes each base once with Push on, and lands there every Ticket that changed it; in a local-only Repository nothing is pushed, and they land there once merged. A failed push or base move in one Repository is tried again on every poll, as above, and the Spec's Tickets wait for it there. A broken base or a missing clone in any Repository the Spec's Tickets changed holds the whole Spec back, since its Spec Branch there can't be checked. Under Pull Request, the Spec Branch is pushed in each Repository, one pull request in each, and the Spec's Tickets land in each Repository as its pull request merges:

```text
acme-shop-14: Accepted under the Branch Landing Method: merge acme-shop-14 into main in Repository api and the Factory lands it
acme-shop-14: merged into main by hand in Repository api
acme-shop-14: landed on main at 4b5c6d7e8f90 in Repository api
acme-shop-18: merged into spec/acme-shop-17 at 3b9e1f0c2d44
acme-shop-18: merged into spec/acme-shop-17 at 5c0d1e2f3a4b in Repository api
acme-shop-17: merged spec/acme-shop-17 into main at 9ad0fd655dd2
acme-shop-17: landing its Tickets: acme-shop-18
acme-shop-18: landed on main at 9ad0fd655dd2
acme-shop-17: merged spec/acme-shop-17 into main at 6e7f8a9b0c1d in Repository api
acme-shop-17: landing its Tickets in Repository api: acme-shop-18
acme-shop-18: landed on main at 6e7f8a9b0c1d in Repository api
acme-shop-19: spec/acme-shop-19 conflicts with the base in acme-shop, api: opened acme-shop-22 to merge the base into it
```

**Mixed Landings.** Each Repository lands by its own Landing, its Own Landing or else the Default Landing, so one Ticket's Changed Repositories can land by different routes. Under Merge the Factory merges the Ticket there, and pushes the base with Push on. Under Branch it waits for you to merge it there. Under Pull Request it pushes the Ticket's branch there for its pull request, and lands it there once that is merged; a Repository under Merge or Branch never gets a pull request. Every Repository is checked before the Ticket lands in any, so a conflict where the Factory merges sends the Ticket back before anything is pushed for a pull request elsewhere, and the Ticket is Partly landed until it has landed in each. With Land per Spec on, a Spec lands in each Repository by that Repository's Landing: its Spec Branch is merged where it is Merge, and pushed for the Spec's pull request where it is Pull Request, every one checked against its base before any is merged or pushed. Under Branch, Land per Spec has no effect there: a Ticket lands there once you merge it, and one that changed only Repositories under Branch doesn't hold its Spec back. Changing a Repository's Own Landing while work is in flight applies from the next landing step in that Repository alone, and what has landed stays. Here the Home lands by Merge and api by Pull Request:

```text
acme-shop-20: landed on main at 1f2e3d4c5b6a
acme-shop-20: pushed acme-shop-20 to origin in Repository api
acme-shop-20: pull request 12 merged in Repository api
acme-shop-20: landed on main in Repository api at 7a8b9c0d1e2f
```

When a base is found for the first time in a run, or changes, the Factory checks that `.mcp.json` is committed on it. At start it refuses to start without the file. Mid-run it cuts no new branches or worktrees, and prints the same message once, until the file is committed; landing carries on.

With **Land per Spec** on, the Spec's accepted Tickets gather on the Spec Branch as under Merge. Once the Spec is ready, the Factory checks the Spec Branch against the fast-forwarded base (a conflict opens the usual Ticket on the Spec), pushes `spec/<spec id>`, and reports that commit for every waiting Ticket: one pull request opens for the Spec, with a section for each Ticket, and each later merge into the Spec Branch is pushed to update it. When it is merged, every waiting Ticket lands at its merge commit and the Spec Branch is deleted:

```text
acme-shop-2: pushed spec/acme-shop-2 to origin
main: fast-forwarded to origin/main at 22eeda3a9de2
acme-shop-5: pull request 9 merged
acme-shop-5: landed on main at 22eeda3a9de2
acme-shop-6: pull request 9 merged
acme-shop-6: landed on main at 22eeda3a9de2
acme-shop-2: deleted spec/acme-shop-2, which is in main
```

The Factory pushes the Spec Branch only over a branch it pushed itself. When the remote already has a branch of that name, say one made by hand, and none of the Spec's Tickets has a pushed commit reported on it there, the Factory doesn't push, forced or not, even under [Rebase](#rebase). It comments once on each waiting Ticket and tries again on every poll, pushing once you have deleted or renamed that branch on the remote (by then the Spec's `branch` Tag can't change):

```text
acme-shop-5: can't push yet: origin already has a branch named spec/acme-shop-2 that the Factory didn't push, so it won't push the Spec Branch over it. Delete or rename it on origin, and the Factory pushes the Spec Branch on a later poll.
```

An Accepted Ticket whose pull request is closed without merging shows **Pull request closed**, offering Send back and Cancel (for a Spec, New fix Ticket and Cancel). A merged pull request that is conflicting shows under **To merge** as conflicting. There is no CI gate and no Land anyway under Pull Request: CI Tracking only informs, and on Azure Pipelines it follows the builds of the pull request's merge.

### Follow-up work on a Spec under review

**New Ticket** adds one implementation Ticket to a Spec from its Spec view, from a Requirement's view, or from a Lane header's Spec popup on the Board. It is Small Change's form: one box whose first line is the title and whose whole text is the description. **Submit ticket** opens the Ticket Ready on that Spec with no Stages, and selects it; **Save as Draft** opens it Draft until you release it. Over HTTP it is the human-only `new_ticket`, which answers like any write:

```text
POST /api/p/acme/new_ticket {"spec":"acme-1","text":"Rename the button\nCall it Save."}
{"id":"acme-2","updated":"2026-11-19T00:19:24Z","changed":["title","state","spec","description","spec_landed","moves","claimable"],"state":"Ready"}
```

The Spec's `ticketable` field says whether the button shows. It is offered on a plain Spec, on a Requirement once its tickets Stage is Done or was Cancelled by Implement, on a Finished Spec, and under every Landing Method. It is hidden on a Map, on a Long-lived Spec, on a Requirement whose Stages are still running, and in an archived Project. Under Pull Request + Land per Spec, a Ticket added while the Spec's pull request is open holds the Spec back from landing until it is in.

The Accepted panel's **To merge** options follow the Landing Method, from the Ticket's `to_merge_options`. Each needs a note:

| Landing Method | To merge offers |
| --- | --- |
| Branch | **Send back to fix**: the next Claim reworks the Ticket on its own branch |
| Branch + Push | **Send back to fix**, unless the CI item lists other Tickets too |
| Pull Request, without Land per Spec | **Send back to fix**, as before |
| Pull Request + Land per Spec | **New fix Ticket** on the Spec's lead Ticket, even if the pull request was closed; no Send back to fix, and nothing on the other Tickets waiting on the Spec |

A refused action shows the core's message in the panel, and New fix Ticket shows a link to the Ticket it opened.

Either Landing Method has a **Push** option, also on the Project Options page, which names a git remote (`origin` by default). Each Repository may name its own Push remote, and each is pushed to, fetched from and cloned under its own.

When it starts, the Factory reports the Home Repository's Push remote to Code Central, read with `git remote get-url`, and again whenever you change the Home's Push remote name. When the Home's clone has no remote of that name, it reports none, and the Home is local-only: nothing is pushed or fetched there, a Ticket lands there once merged, and the CI gate never holds there, while the other Repositories push and gate as usual. A missing remote stops nothing. A remote you add to the Home's clone later is reported at the Factory's next start.

A Repository other than the Home that has a Clone URL but whose clone lacks its Push remote is Repository missing, which holds back only the Tickets landing there.

A Home that loses its remote while it lands by Pull Request is local-only under Pull Request. The Factory says so once, and nothing lands there until you give it another Landing or add the remote and restart the Factory:

```text
warning: Repository acme-shop is local-only under Pull Request: nothing lands in it until you give it another Landing in Project Options, or add its Push remote, origin, to its clone and restart the Factory
```

Under **Merge + Push**, after merging a Ticket into the base the Factory runs `git push <remote> <base>`, and lands the Ticket only once the push succeeds, so Done means the work is on the remote:

```text
acme-shop-5: pushed main to origin
acme-shop-5: landed on main at f90edae47659
```

Pushing the base sends everything on it, so commits of your own on the base that you haven't pushed go to the remote too.

Under Merge + Push, before it lands anything or cuts a branch, the Factory fetches the remote and brings the local base level with it, whether or not CI Tracking is on. A base behind the remote's, say after a push from another clone, is fast-forwarded, and new branches are cut from the new tip:

```text
main: fast-forwarded to origin/main at 22eeda3a9de2
```

A base level with the remote's, or ahead of it, as it is after the Factory's own merge that isn't pushed yet, is current. When both have moved, say a push from another clone landed while the Factory's own push was failing, the Factory moves the local base onto the remote's tip and then pushes. Its own unpushed merges are rebuilt on the new tip, each with the Ticket's branch (or the Spec Branch) untouched as its second parent and its message kept, so every merged Ticket's head is still in the base and the Ticket lands as it would have; your own unpushed commits on the base are replayed on top, in order:

```text
main: moved 2 unpushed commits onto origin/main, now at 7c1e0b2d9a4f
acme-shop-5: pushed main to origin
acme-shop-5: landed on main at 7c1e0b2d9a4f
```

The move is all or nothing: the base moves only once every commit applied cleanly. If a rebuilt merge or a replayed commit conflicts, or an unpushed merge on the base isn't one of the Factory's (its second parent is neither a Ticket's branch nor a Spec Branch), the local base is left exactly as it was and the Factory reports [Base diverged](#pull-request), landing nothing until you reconcile them:

```text
warning: can't move main onto origin/main: 9ad0fd655dd2 conflicts in config.yaml
warning: the base, main, has diverged from the remote's: it is at 9ad0fd655dd2 and the remote's at 22eeda3a9de2. Nothing lands and no branch is cut until you reconcile them by hand
```

If uncommitted changes in the checkout where the base is checked out are in the way of a fast-forward or a move, the poll waits and warns. A failed fetch is a warning and a retry on the next poll, as under Pull Request.

The Factory never force-pushes and never rewrites a pushed commit; under Branch it never pulls or rebases either (under [Pull Request](#pull-request) it fetches and fast-forwards the base). When the push fails, say because the remote is unreachable, the Ticket stays Accepted, with the base already merged, and the Factory comments once, quoting git:

```text
acme-shop-5: can't land yet: couldn't push main to origin
```

> The Factory can't land this yet: couldn't push main to origin:
>
> ```
> fatal: unable to access 'https://example.com/acme-shop.git/': Could not resolve host: example.com
> ```
>
> It stays Accepted and the Factory tries again on every poll.

On every poll it tries again: it finds the work already in the base, checks whether the remote's base (its `origin/main` remote-tracking ref) holds it, and pushes again if not. A remote that has moved ahead is left for you to sort out: pull it into your base, and the next push goes through. An In Review Ticket you merge by hand lands under the same rule, only once the base with it in is pushed.

Under **Branch + Push**, once you accept a Ticket the Factory runs `git push <remote> <branch>`, so you can merge the branch on the host. It pushes only accepted work: a Ticket In Review is never pushed, so work you reject never leaves your machine. The Ticket stays Accepted until you merge the branch into the base, as under Branch:

```text
acme-shop-5: pushed acme-shop-5 to origin
acme-shop-5: Accepted under the Branch Landing Method: merge acme-shop-5 into main and the Factory lands it
```

Before pushing, it checks the branch's remote-tracking ref (`origin/acme-shop-5`): when that already holds the Ticket's commit, there's nothing to push. When the push fails, the Ticket stays Accepted and the Factory comments once for each distinct reason, quoting git, and tries again on every poll:

```text
acme-shop-5: can't push yet: couldn't push acme-shop-5 to origin
```

> The Factory can't push this Ticket's branch yet: couldn't push acme-shop-5 to origin:
>
> ```
> fatal: unable to access 'https://example.com/acme-shop.git/': Could not resolve host: example.com
> ```
>
> It stays Accepted and the Factory tries again on every poll.

A branch you merge into the base by hand lands as under Branch, whether or not it was pushed. If you merge it on the host instead, pull the base, and the Factory lands it on its next poll.

### With CI Tracking

With [CI Tracking](setting-up-a-project.md#watching-ci) on as well, the Factory reports each push it makes for a Ticket as the Ticket's pushed commit, once: the base's commit on the remote under Merge + Push, the branch's under Branch + Push. The CI Watcher then follows the runs GitHub starts for it.

Under **Merge + Push**, CI gates landing, so Done means on the remote and green. The Factory merges, pushes and reports the pushed commit, and leaves the Ticket Accepted. On every later poll it reads the Ticket's CI result, and once it is passed, lands the Ticket at the pushed commit that passed. A Ticket with several pushed commits since it was last sent back passes as soon as any of them does, an earlier one that goes green after a later one failed included:

```text
acme-shop-5: pushed main to origin
acme-shop-5: CI tracks main at 9c41d07a2b18
acme-shop-5: waiting on CI at 9c41d07a2b18 before it lands
acme-shop-5: landed on main at 9c41d07a2b18
```

There is no State for waiting on CI: the Ticket stays Accepted, so a Ticket it blocks isn't claimed until its CI has passed and it lands. Waiting doesn't hold up the rest: the Factory goes on merging and pushing other accepted Tickets meanwhile. An In Review Ticket you merge by hand waits the same way, In Review.

A later commit on the remote's base contains the work of every Ticket pushed before it, so the Factory reports it to them. On each poll, once the base is synced, it compares the remote's base tip with the latest pushed commit of each Ticket waiting on CI: when the tip is a different commit and the Ticket's pushed commit is an ancestor of it, the tip becomes the Ticket's newest pushed commit. That happens after another Ticket's merge is pushed, and after a commit pushed by hand from another clone. When the tip passes, every Ticket that was waiting lands at it:

```text
acme-shop-5: pushed main to origin
acme-shop-5: CI tracks main at 9c41d07a2b18
acme-shop-6: pushed main to origin
acme-shop-6: CI tracks main at 4be0d2f713a6
acme-shop-5: CI tracks main at 4be0d2f713a6
acme-shop-5: landed on main at 4be0d2f713a6
acme-shop-6: landed on main at 4be0d2f713a6
```

A Ticket whose pushed commit isn't in the tip, say after the base was force-pushed back, gets no report.

While its CI is running, an Accepted Ticket waiting on it doesn't show in Needs Me, however long CI takes. When no word of its CI comes for 10 minutes after its push, its pushed commit shows in Needs Me as **CI unavailable**, so a broken CI Watcher doesn't hold up the work unseen. CI unavailable, CI failed and CI cancelled are each one Needs Me item per pushed commit, with no Ticket of its own: the item lists every Ticket waiting on the commit, across Specs, since under Merge + Push several Tickets come to share the base's tip. Its detail says which cause applies: the CI Watcher isn't Watching (with its error, which the CI section of Project Options also shows), or the CI Watcher is Watching but no CI Runs have been reported for the pushed commit, say because no workflow runs on the base or all that do are Ignored workflows:

```text
CI Watcher not Watching: gh is logged out · 9c41d07 pushed 12m ago
No CI Runs reported for 9c41d07 pushed 12m ago
```

The item leaves Needs Me once runs are reported or the CI Watcher is Watching again, or when the gate lifts.

When the CI result is failed, the Ticket stays Accepted, and the Factory says so once:

```text
acme-shop-5: CI failed at 9c41d07a2b18, so it stays Accepted
```

The pushed commit shows in Needs Me as **CI failed**, one item listing every Ticket waiting on it, with one Notification, until you choose what to do. The item's panel, in the Inbox and in Focus Mode, shows the CI section and the failure options; each listed Ticket's own view shows its CI result and links the item. Your choice is stored once for the commit and acts on every listed Ticket. **Land anyway** accepts the failure, say a flake unrelated to the work, and the Factory lands each of them at the pushed commit on its next poll:

```text
acme-shop-5: landing anyway at 9c41d07a2b18, as you chose, though its CI failed
acme-shop-5: landed on main at 9c41d07a2b18
```

Land anyway holds for that pushed commit only: if a listed Ticket is pushed again, the new push's CI result decides.

**Re-run failed jobs**, for a failure that may be a flake, asks the CI Watcher to re-run each failed run's failed jobs, once for the commit. The item leaves Needs Me and every listed Ticket's CI result reads running at once; on its next poll the CI Watcher asks GitHub for the re-run, then reports each run's new attempt, and the Tickets land if the new attempts pass. If one fails again, the commit is CI failed again, with another Notification. If GitHub refuses the re-run, say for a run too old to re-run, the request lapses after 10 minutes and the commit is CI failed again, so you can choose another option.

The last two options fix the failure forward. Under Merge + Push the failed work is already on the remote's base, so neither reverts it. Each carries a note you can edit, which Code Central prefills with a run summary: the failed run's link, its failed jobs' names, the `gh run view <run> --log-failed` command for their logs, and the last commit that passed CI in that Repository when it knows one. **New fix Ticket**'s note adds the Project's Fix instructions after the run summary; **Send back to fix**'s is the run summary alone:

- **Send back to fix** sends the Ticket back to Ready with the note. It is offered only when the item lists exactly one Ticket, and not a Spec-landed one: a commit several Tickets wait on has no one branch to fix it on. The next Claim works on the same branch, in the same worktree, and finishes it for you to review again. When you accept it, the Factory merges and pushes it again, and the new pushed commit's CI result decides; the old one's runs stay on the Ticket, folded under "Earlier pushes".
- **New fix Ticket** opens a Ready Ticket on the Project's **CI Fixes** Spec, a Long-lived Spec Code Central creates the first time you use it, titled `Fix the CI failure of acme-shop-5: …`, with a line naming the failed Ticket and then the note as its description. It doesn't accept the failed CI result: the listed Tickets stay Accepted and the item leaves Needs Me while the fix Ticket is open, and when the fix's push passes CI the listed Tickets land with it. If you cancel the fix Ticket while the commit is still failing, the item returns. An agent picks up the fix Ticket, and a comment on the Ticket you chose it on names it.

The gate holds only while Merge, Push and CI Tracking are all on. Turning CI Tracking off, turning Push off, or switching to Leave as a branch lifts it, and the Factory lands every Ticket waiting on CI on its next poll.

A cancelled run, say one you or GitHub cancelled, is not a failure, so the web UI shows it in neutral grey, labelled **cancelled**, in the CI view, the Ticket's CI section and its CI badge, where a failed run stays red. The Ticket stays Accepted, the Factory prints `CI cancelled at …, so it stays Accepted`, and the pushed commit shows in Needs Me as **CI cancelled**, one item listing the Tickets waiting on it, with no failure-specific Notification, offering **Re-run**, **Land anyway** (under Merge + Push) and **Send back to fix** (for one Ticket alone), but no New fix Ticket. Nothing re-runs by itself: Re-run asks the CI Watcher to re-run the cancelled run, and the listed Tickets' CI result reads passed if the new attempt passes.

Under **Branch + Push**, CI only informs you: the Factory lands the Ticket when it sees your merge, whatever its CI result, so merging stays your decision. A failed CI result still shows the pushed commit in Needs Me as **CI failed**, listing the Ticket, with Re-run failed jobs, Send back to fix and New fix Ticket but without Land anyway: merging the branch is how you land it anyway.
