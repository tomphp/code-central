# Running the Factory

This guide takes one Requirement through every Stage with `code-central-factory`, from submitting it to seeing its implementation land. It follows the same Requirement as [Working with Claude Code](working-with-claude-code.md), on `~/src/acme-shop` set up as in [Setting up a Project](setting-up-a-project.md), so you can compare the two ways. The prompts and output shown are copied from a real run, cut with `…` where they are long. The agents' wording, their Questions and how many implementation Tickets they make will differ from run to run; the lines the Factory prints, the States and the buttons won't.

The run started from a freshly set up `acme-shop`, so its IDs start at `acme-shop-1` again. If you've already worked the Requirement by hand, yours will be higher.

The Factory is a program you leave running in a terminal. It claims the Project's Ready Tickets, runs one `claude -p` session per Claim, and resumes a session when you answer its Round. Once you accept an implementation Ticket, it lands it by the Project's Landing Method: under **Merge**, the default, it merges the branch into the base and lands it; under **Branch** it leaves the branch for you to merge, and lands the Ticket once you have (see [How accepted work lands](#how-accepted-work-lands)). What's left to you is what a Factory never does:

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

   `code-central-factory -h` lists its flags. These are the ones that matter:

   | Flag | What it does |
   | --- | --- |
   | `--parallel <n>` | The most `claude` turns running at once. The default is 3. |
   | `--base <branch>` | The branch work starts from, and the one it merges into when it lands. The default is the branch checked out when you start it, here `main`. |
   | `--model <model>` | The model for every session it starts or resumes, such as `--model opus`, unless the Ticket sets one. The default is Claude Code's own. See [Model and effort](#model-and-effort). |
   | `--effort <level>` | The effort for every session it starts or resumes, such as `--effort high`, unless the Ticket sets one. The default is Claude Code's own. It must be a level `claude --help` lists; see [What `claude` accepts](#what-claude-accepts). |
   | `--bypass-permissions` | Runs every session in `bypassPermissions` mode, with no permission checks. It prints a warning line at the start. |

   Without `--bypass-permissions`, every session runs in auto mode with no permission prompts. Anything auto mode won't allow is denied and the turn carries on without it. If a turn ends on denied calls, the Factory posts a Round asking you about each one, and answering it lets the session try again. Allow rules for the repository go in its committed `.claude/settings.json`.

   The first Ctrl-C drains: the Factory claims, resumes and lands nothing more, lets the running turns finish, then exits. A second Ctrl-C stops the running turns. Either way the Claims are kept, and the Factory picks them up when you start it again. Step 8 shows it.

   Check: the Factory is still running, and its only line is `base: main`.

2. **Submit the Requirement from the UI.** Open <http://localhost:7733>, click **+ New**, type the same Requirement's text and click **Submit requirement**:

   ```text
   Format prices for display

   Add a Go function that turns a price in pence into the string a customer sees, so 1999 shows as £19.99 and 5 shows as £0.05.
   ```

   The UI opens the Requirement, `acme-shop-1 Format prices for display`, with its grilling Ticket, `acme-shop-2`, in Ready and *Waiting for a Factory to claim Grilling. Nothing needs you yet.* Within a poll, the Factory claims it:

   ```text
   acme-shop-2 claimed (grilling): "Grilling: Format prices for display" in Spec acme-shop-1; worktree /Users/you/src/acme-shop.worktrees/acme-shop-2; resume: claude --resume 37f7978d-a171-4aee-b737-220845e8b7a8
   ```

   Each Claim gets a worktree of its own under `~/src/acme-shop.worktrees`, named after the Ticket. A Stage's worktree is read-only and detached at the base's tip; the Stages write to Code Central, not to the repository.

   The session's prompt is the Stage's skill and the Ticket, `/grilling acme-shop-2`. The rest of what the session needs, its Ticket and Claim, how the Factory runs it and the Stage Guide, goes once into its system prompt (`--append-system-prompt`), which every resume passes again so a session keeps it after a compaction.

   Check: `acme-shop-2` is In Progress, and the Requirement's row reads *Grilling: a Factory is on it*. Nothing needs you yet.

   **Map**, in the **▾** menu beside **+ New**, starts a Map from the same dialog instead. Its one Ready Ticket, *Charting: …*, is claimed within a poll like a grilling Ticket, in a read-only Stage worktree, and grills you on the Map's destination through Rounds. The Map's row in the **Requirements** view counts its decisions, such as *0/1 decided*, in place of the Stage strip. The same **+ New ▾** is in Focus Mode's header, beside **Exit**, and submitting there keeps you in Focus Mode, on the item you were on, with a brief "Requirement added" or "Map added" note.

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

   There is nothing to tell a session. Here the session had left `code-central wait` running, so it picked the answers up itself. If its turn had ended instead, the Factory would resume it with the answers on its next poll, and print a line such as `acme-shop-2 resumed (grilling): "Grilling: Format prices for display" with the answers to Round 1`. If the answers raise new Questions, another Round comes. Answer it the same way.

   Check: **Needs Me** is back to 0, and the Factory prints the turn's end:

   ```text
   acme-shop-2 turn ended (grilling): "Grilling: Format prices for display": exit 0, success (completed); now In Review; ran 2m21s
   ```

5. **Review and accept the grilling Stage.** `acme-shop-2` is In Review, with the session's summary in *Review the finished work*:

   ```text
   Grilling took one Round of six Questions, and every recommendation was accepted. The full write-up is in the resolution comment on this Ticket. The Spec is retitled "Display string for a Price in pence", and its body is unchanged.

   Key decisions:
   - A type Price int64 in pence, whose String() method gives the display price, in package price under a new acme-shop module.
   …
   ```

   Read it, and the resolution comment in the Ticket's history, then click **Accept**. To send it back, write what to change in *What to change (required to reject)* and click **Reject**: the Ticket goes back to Ready, and the Factory claims it again on its next poll. Your note is kept as a comment on the Ticket, for the new session to work from.

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

   Check the Spec's body on `acme-shop-1`. The spec Stage's review offers **Split into Tickets** and **Implement**, and the summary ends with a recommendation and a one-line reason based on the Spec's size. **Implement** builds the Spec as one implementation Ticket and cancels the tickets Stage, which the strip shows as skipped, so the Factory goes straight to building it. Here, click **Split into Tickets** on `acme-shop-3`. The Factory claims the tickets Stage, `acme-shop-4`:

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

   Its summary names the branch and head commit, the acceptance criteria met, the test result and what to check:

   ```text
   Where

   Branch acme-shop-5, head 188e2297b82e50d6884e956984042d34d4f88d78 (2 commits on top of 05856b0, not pushed).
   …
   Tests

   go test ./...: ok acme-shop/price (12 cases, all pass).
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

   Then click **Accept**. This run uses the default Landing Method, Merge, so you don't merge or land: within a poll, the Factory merges the branch into `main`, lands the Ticket, removes its worktree, and claims `acme-shop-6`, which was waiting on it:

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

   You can still merge an In Review Ticket's branch yourself: the Factory notices it on its next poll and lands the Ticket.

   Check: both Tickets are Done, Build shows *2/2*, and the Requirement's row reads *Finished · 2 of 2 Tickets Done*.

8. **Stop the Factory.** Press Ctrl-C in its terminal. With no turns running, it drains and exits at once:

   ```text
   ^C
   draining: no more claiming, resuming or landing; Ctrl-C again to stop the running turns (Claims are kept)
   drained: no turns running
   ```

   With turns running, it prints `draining: waiting on …` and exits once they finish, or stops them at a second Ctrl-C, as in step 1.

   Check: the Factory has exited, and the Requirement is still *Finished*.

## The Factory Status dot

While `code-central-factory` runs, it sends Code Central a Heartbeat on every poll, and the Project's row in the left pane shows a dot before its name: green for Online, amber for Draining and grey for Offline. Hover or focus the dot to list each Factory as name on host, its status, its last Heartbeat and its note. Project Options has the same list under **Factories**. Archived Projects show no dot.

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

## Model and effort

Before every turn, a first turn or a resume after answers, after a cut-off or after a restart, the Factory works out the model and effort the turn runs with. Each is worked out on its own, highest first:

1. the Ticket's `model` or `effort` Tag;
2. the Project's Models row for the Ticket's kind of work, its Stage Guide name;
3. the Project's default Models row;
4. the Factory's `--model` or `--effort` flag;
5. Claude Code's own default: the Factory passes no flag.

Set tiers 2 and 3 in the [Models section of Project Options](setting-up-a-project.md#choosing-models-for-each-kind-of-work): a Project default row, and a row for each kind of work. A Ticket's kind of work comes from its Labels: `grilling`, `spec` or `tickets` for a Stage, `map-chart`, `map-grilling`, `map-research` or `map-task` for a Map Ticket, `prototype` for a Prototype, and `implementation` for any other Ticket. Override them for one Ticket with its Tags, from the Ticket panel, from an agent with `set_tags`, or by hand:

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

## Map Tickets and Prototypes

A Map's Tickets each carry a Wayfinder Label, and the Factory claims them like a Stage's Ticket, by the skill its Label names. A Prototype is claimed by its skill too:

| Label | Guide | Skill |
| --- | --- | --- |
| `wayfinder:chart` | `map-chart` | `/wayfinder` |
| `wayfinder:grilling` | `map-grilling` | `/grilling`, with `/domain-modeling` alongside |
| `wayfinder:research` | `map-research` | `/research` |
| `wayfinder:task` | `map-task` | none: the session starts from the Guide alone |
| `prototype` | `prototype` | `/prototype` |

A Map's Charting, grilling and task Tickets each run in a read-only worktree detached at the base's tip, as a Stage does, and write to Code Central, not to the repository. Each one's line names its Guide in place of the Stage:

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

A Ticket with any of these Labels is never run as an implementation Ticket.

Selecting the Map in the **Requirements** view opens the Map view: **New Prototype** and its Destination, then its Tickets as a graph read left to right, each Blocker before what it blocks, coloured like the Stage strip, and a dim **Not yet specified** node at the right edge that shows that section when clicked. A Draft Ticket that nothing blocks is on the frontier: click **Start** on its node to approve it to Ready, and the Factory claims it within a poll. Clicking a node opens its Ticket below. **Decisions so far** and **Out of scope** sit beside the graph.

To let the Factory find the way for you, tick **Guided** in the Map view's header. It sets the Map's `guided` Tag to `on`, and from then on, on each poll, the Factory checks whether the Map is idle: every Ticket on it that isn't research is Draft, Done or Cancelled. When it is, the Factory approves the Map's first frontier Ticket in created order, one per poll, and claims it as usual:

```text
acme-shop-12 approved (guided Map acme-shop-8): "Which store?"
acme-shop-12 claimed (map-grilling): "Which store?" in Spec acme-shop-8; worktree /Users/you/src/acme-shop.worktrees/acme-shop-12; resume: claude --resume …
```

Any Ticket on the Map that is Ready, In Progress, Needs Answer, Answered, Failed, In Review or Accepted holds it, so you are never asked two things about one Map at once; a research Ticket running in the background doesn't. The Factory starts nothing while it is paused for an outage or draining. Untick **Guided** to remove the Tag and hand the Map back to your **Start** buttons.

Once every Ticket on the Map is Done or Cancelled, the Map view's header has **Write the Spec**. It opens the **New requirement** dialog with the Map's title, a link to the Map and its Destination already filled in. Edit the text if you like, then click **Submit requirement**. Code Central creates a new Requirement with only the spec and tickets Stages, and its spec Stage is Ready straight away. Its spec Stage's review has the same choice, **Split into Tickets** or **Implement**. The Factory claims it within a poll like any spec Stage, and the agent works from the Map's Decisions so far and the resolution comments they link, in place of a grilling Ticket. The Requirement's strip shows a done **Map** segment where Grilling would be, linking back to the Map. The Map stays as the record, and its header links each Requirement written from it. From a terminal, `code-central submit-requirement --map <map> --text "…"` does the same.

When a Map's Ticket asks you a Round or finishes into In Review, it waits on you in **Needs Me** and Focus Mode as its own entry, not folded into a row as a Stage Ticket is. Its row reads `↳ Map: Dark mode` under its title, and its Focus Item reads *Map: Dark mode*. A Prototype also carries a **Prototype** badge, and on a plain Spec its row names that Spec instead.

## How accepted work lands

Each Project has a **Landing Method**, which you set on its **Project Options** page, from the three-dots menu on the Project. The Factory reads it again on every poll, before it lands anything, so a change applies from its next poll to every Ticket still waiting to land, with no restart.

- **Merge** ("Merge into base"), the default, is what step 7 shows. The Factory merges an accepted Ticket's branch into the base and lands it. When the branch conflicts with the base it sends the Ticket back to Ready; when it can't move the base it leaves the Ticket Accepted with a comment and tries again on every poll. It keeps an Accepted Ticket's worktree until the Ticket lands, since a Ticket sent back is worked in it again.
- **Branch** ("Leave as a branch") leaves the merging to you. The Factory never merges an Accepted Ticket and never sends it back for conflicts: a conflict in a branch you merge is yours to resolve. Within a poll of your **Accept**, it says so once and removes the Ticket's worktree, keeping its branch, so you can check the branch out:

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

### Land per Spec

Under Merge with Land per Spec on, the accepted work of every Spec but a Long-lived one first gathers on its Spec Branch, `spec/<spec id>`. A Long-lived Spec is one labelled `long-lived`, as a CI Rule's Spec and the Issues Spec are; its Tickets land one by one. Once every implementation Ticket on the Spec is Accepted, Done or Cancelled (and, for a Requirement, its tickets Stage is finished), and at least one is merged into the Spec Branch, the Factory merges the Spec Branch into the base as `Merge spec <spec id>: <title>`, and lands every waiting Ticket at that commit. With Push on, it pushes the base once and lands them after the push succeeds; with CI Tracking on, the same pushed commit is reported for each and they all land once its CI is passed. Only the base is pushed.

```text
acme-shop-5: merged into spec/acme-shop-2 at 3b9e1f0c2d44
acme-shop-2: merged spec/acme-shop-2 into main at 9ad0fd655dd2
acme-shop-2: landing its Tickets: acme-shop-5, acme-shop-6
acme-shop-5: landed on main at 9ad0fd655dd2
acme-shop-6: landed on main at 9ad0fd655dd2
```

When the Spec Branch conflicts with the base, the Factory opens a Ready Ticket on the Spec, "Merge the base into the Spec Branch of <spec id>", and retries once it is accepted.

Turning Land per Spec off part-way makes the Factory's next poll merge every Spec Branch that holds merged-but-not-landed Tickets into the base, ready or not, and land them as a group; later Tickets land one by one. Adding the `long-lived` Label to a Spec while its Tickets wait does the same for that Spec alone. Turning Land per Spec on, or taking the Label off, affects only Tickets accepted afterwards. A Ticket added to a Spec after it has landed lands with a second batch: the Spec Branch is merged into the base again once the new Ticket is accepted.

A Spec Branch is deleted once it is in the base and every Ticket on its Spec is Done or Cancelled. One that isn't in the base, as when the whole Spec is cancelled, is kept, and the Factory says so once. Ticket branches are never deleted.

```text
acme-shop-2: deleted spec/acme-shop-2, which is in main
acme-shop-4: every Ticket is finished but spec/acme-shop-4 isn't in main: kept
```

### Pull Request

Under **Pull Request**, the Project's Push remote must be on GitHub or Azure Repos, and Push is implied: the Factory pushes each Ticket's branch to the remote once you accept it, and reports the pushed commit and branch to Code Central. The Pull Request Watcher (see [The Pull Request Watcher](running-as-a-service.md#the-pull-request-watcher)) opens a ready-for-review pull request into the base from that branch, with the Ticket's title and finish summary, and reports its state. You merge it on the host, by any of its merge buttons. The Factory never calls the host and never merges the Ticket itself:

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

**Base diverged.** When the local base has commits the remote's lacks, it can't be fast-forwarded. The Factory reports it, then lands nothing and claims no work that would cut a branch, until you reconcile the local base with the remote's, for example by resetting or rebasing it. Its next poll sees the fix by itself. Code Central shows a **Base diverged** item in Needs Me and Focus Mode, with both commits:

```text
warning: the base, main, has diverged from the remote's: it is at 9ad0fd655dd2 and the remote's at 22eeda3a9de2. Nothing lands and no branch is cut until you reconcile them by hand
main: no longer diverged from the remote's
```

**Base missing.** Under every Landing Method, a base that exists neither locally nor on the Push remote is reported as missing through `report_base`. Code Central shows one **Base missing** item per Project in Needs Me and Focus Mode, naming the branch and the fix: create the branch, or change the Base branch in Project Options. The report clears when a later poll reports the base current.

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

An Accepted Ticket whose pull request is closed without merging shows **Pull request closed**, offering Send back and Cancel (for a Spec, New fix Ticket and Cancel). A merged pull request that is conflicting shows under **To merge** as conflicting. There is no CI gate and no Land anyway under Pull Request: CI Tracking only informs, and on Azure Pipelines it follows the builds of the pull request's merge.

### Push

Either Landing Method has a **Push** option, also on the Project Options page, which names a git remote (`origin` by default). The Factory checks that its repository has that remote, with `git remote get-url`. It refuses to start when it doesn't:

```console
$ code-central-factory
code-central-factory: refusing to start: Push names the remote "upstream", which this repository doesn't have: add it with `git remote add`, or change it on the Project's Project Options page in Code Central
```

If you change the setting to a missing remote while the Factory runs, it warns once and lands nothing, not even a Ticket you merged by hand, until you add the remote or change the setting back. It goes on claiming and resuming work meanwhile:

```text
warning: nothing lands until this is fixed: Push names the remote "upstream", which this repository doesn't have: add it with `git remote add`, or change it on the Project's Project Options page in Code Central
```

Under **Merge + Push**, after merging a Ticket into the base the Factory runs `git push <remote> <base>`, and lands the Ticket only once the push succeeds, so Done means the work is on the remote:

```text
acme-shop-5: pushed main to origin
acme-shop-5: landed on main at f90edae47659
```

Pushing the base sends everything on it, so commits of your own on the base that you haven't pushed go to the remote too.

The Factory never pulls, rebases or force-pushes under Merge or Branch (under [Pull Request](#pull-request) it fetches and fast-forwards the base). When the push fails, say because the remote is unreachable or has moved ahead of your base, the Ticket stays Accepted, with the base already merged, and the Factory comments once, quoting git:

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

Under **Merge + Push**, CI gates landing, so Done means on the remote and green. The Factory merges, pushes and reports the pushed commit, and leaves the Ticket Accepted. On every later poll it reads the Ticket's CI result, and once it is passed, lands the Ticket at its pushed commit:

```text
acme-shop-5: pushed main to origin
acme-shop-5: CI tracks main at 9c41d07a2b18
acme-shop-5: waiting on CI at 9c41d07a2b18 before it lands
acme-shop-5: landed on main at 9c41d07a2b18
```

There is no State for waiting on CI: the Ticket stays Accepted, so a Ticket it blocks isn't claimed until its CI has passed and it lands. Waiting doesn't hold up the rest: the Factory goes on merging and pushing other accepted Tickets meanwhile, and each lands on its own pushed commit's CI result, not on a later push's. An In Review Ticket you merge by hand waits the same way, In Review.

While its CI is running, an Accepted Ticket waiting on it doesn't show in Needs Me, however long CI takes. When no word of its CI comes for 10 minutes after its push, it shows in Needs Me as **CI unavailable**, so a broken CI Watcher doesn't hold up the work unseen. Its detail says which cause applies: the CI Watcher isn't Watching (with its error, which the CI section of Project Options also shows), or the CI Watcher is Watching but no CI Runs have been reported for the pushed commit, say because no workflow runs on the base or all that do are Ignored workflows:

```text
CI Watcher not Watching: gh is logged out · 9c41d07 pushed 12m ago
No CI Runs reported for 9c41d07 pushed 12m ago
```

It leaves Needs Me once runs are reported or the CI Watcher is Watching again, or when the gate lifts.

When the CI result is failed, the Ticket stays Accepted, and the Factory says so once:

```text
acme-shop-5: CI failed at 9c41d07a2b18, so it stays Accepted
```

It shows in Needs Me as **CI failed**, with one Notification, until you choose what to do. Its panel, in the Inbox and in Focus Mode, shows its CI section and the failure options. **Land anyway** accepts the failure, say a flake unrelated to the work, and the Factory lands the Ticket at its pushed commit on its next poll:

```text
acme-shop-5: landing anyway at 9c41d07a2b18, as you chose, though its CI failed
acme-shop-5: landed on main at 9c41d07a2b18
```

Land anyway holds for that pushed commit only: if the Ticket is pushed again, the new push's CI result decides.

**Re-run failed jobs**, for a failure that may be a flake, asks the CI Watcher to re-run each failed run's failed jobs. The Ticket leaves Needs Me and its CI result reads running at once; on its next poll the CI Watcher asks GitHub for the re-run, then reports each run's new attempt, and the Ticket lands if the new attempts pass. If one fails again, the Ticket is CI failed again, with another Notification. If GitHub refuses the re-run, say for a run too old to re-run, the request lapses after 10 minutes and the Ticket is CI failed again, so you can choose another option.

The last two options fix the failure forward. Under Merge + Push the failed work is already on the remote's base, so neither reverts it. Both carry a note, prefilled with the failed run's link, its failed jobs' names and the `gh run view <run> --log-failed` command for their logs:

- **Send back to fix** sends the Ticket back to Ready with the note. The next Claim works on the same branch, in the same worktree, and finishes it for you to review again. When you accept it, the Factory merges and pushes it again, and the new pushed commit's CI result decides; the old one's runs stay on the Ticket, folded under "Earlier pushes".
- **New fix Ticket** opens a Ready Ticket on the same Spec, titled `Fix the CI failure of acme-shop-5: …`, with the note as its description, and accepts the failed CI result as Land anyway does: the Factory lands the failed Ticket on its next poll, and an agent picks up the fix Ticket. A comment on the failed Ticket names the fix Ticket.

The gate holds only while Merge, Push and CI Tracking are all on. Turning CI Tracking off, turning Push off, or switching to Leave as a branch lifts it, and the Factory lands every Ticket waiting on CI on its next poll.

A cancelled run, say one you or GitHub cancelled, is not a failure, so the web UI shows it in neutral grey, labelled **cancelled**, in the CI view, the Ticket's CI section and its CI badge, where a failed run stays red. The Ticket stays Accepted, the Factory prints `CI cancelled at …, so it stays Accepted`, and it shows in Needs Me as **CI cancelled**, with no failure-specific Notification, offering **Re-run**, **Land anyway** (under Merge + Push) and **Send back to fix**, but no New fix Ticket. Nothing re-runs by itself: Re-run asks the CI Watcher to re-run the cancelled run, and the Ticket's CI result reads passed if the new attempt passes.

Under **Branch + Push**, CI only informs you: the Factory lands the Ticket when it sees your merge, whatever its CI result, so merging stays your decision. A failed CI result still shows the Ticket in Needs Me as **CI failed**, with Re-run failed jobs, Send back to fix and New fix Ticket but without Land anyway: merging the branch is how you land it anyway.
