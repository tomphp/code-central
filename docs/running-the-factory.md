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
   | `--model <model>` | The model passed to every session it starts or resumes, such as `--model opus`. The default is Claude Code's own. |
   | `--bypass-permissions` | Runs every session in `bypassPermissions` mode, with no permission checks. It prints a warning line at the start. |

   Without `--bypass-permissions`, every session runs in auto mode with no permission prompts. Anything auto mode won't allow is denied and the turn carries on without it. If a turn ends on denied calls, the Factory posts a Round asking you about each one, and answering it lets the session try again. Allow rules for the repository go in its committed `.claude/settings.json`.

   The first Ctrl-C drains: the Factory claims, resumes and lands nothing more, lets the running turns finish, then exits. A second Ctrl-C stops the running turns. Either way the Claims are kept, and the Factory picks them up when you start it again. Step 8 shows it.

   Check: the Factory is still running, and its only line is `base: main`.

2. **Submit the Requirement from the UI.** Open <http://localhost:7733>, click **New requirement**, type the same Requirement's text and click **Submit requirement**:

   ```text
   Format prices for display

   Add a Go function that turns a price in pence into the string a customer sees, so 1999 shows as £19.99 and 5 shows as £0.05.
   ```

   The UI opens the Requirement, `acme-shop-1 Format prices for display`, with its grilling Ticket, `acme-shop-2`, in Ready and *Waiting for a Factory to claim Grilling. Nothing needs you yet.* Within a poll, the Factory claims it:

   ```text
   acme-shop-2 claimed (grilling): "Grilling: Format prices for display" in Spec acme-shop-1; worktree /Users/you/src/acme-shop.worktrees/acme-shop-2; resume: claude --resume 37f7978d-a171-4aee-b737-220845e8b7a8
   ```

   Each Claim gets a worktree of its own under `~/src/acme-shop.worktrees`, named after the Ticket. A Stage's worktree is read-only and detached at the base's tip; the Stages write to Code Central, not to the repository.

   Check: `acme-shop-2` is In Progress, and the Requirement's row reads *Grilling: a Factory is on it*. Nothing needs you yet.

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

   Check the Spec's body on `acme-shop-1`, then **Accept** `acme-shop-3`. The Factory claims the tickets Stage, `acme-shop-4`:

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

   - If the branch conflicts with the base, it sends the Ticket back to Ready with a comment naming the conflicting files, and its next Claim merges the base in first.
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

## Map Tickets and Prototypes

A Map's Tickets each carry a Wayfinder Label, and the Factory claims them like a Stage's Ticket, by the skill its Label names. A Prototype is claimed by its skill too:

| Label | Guide | Skill |
| --- | --- | --- |
| `wayfinder:chart` | `map-chart` | `/wayfinder` |
| `wayfinder:grilling` | `map-grilling` | `/grilling`, with `/domain-modeling` alongside |
| `wayfinder:task` | `map-task` | none: the prompt is the Guide alone |
| `prototype` | `prototype` | `/prototype` |

Each Map Ticket runs in a read-only worktree detached at the base's tip, as a Stage does, and writes to Code Central, not to the repository. Its line names its Guide in place of the Stage:

```text
acme-shop-9 claimed (map-chart): "Charting: Dark mode" in Spec acme-shop-8; worktree /Users/you/src/acme-shop.worktrees/acme-shop-9; resume: claude --resume …
```

A **Prototype** is a Ticket labelled `prototype`, on any Spec. Submit one with `code-central submit-prototype <spec> --text "…"` (or `submit_prototype` over MCP); its first line becomes the title, after "Prototype: ". The Factory claims it with `/prototype` and runs it in a worktree of its own, as for implementation, but on a local branch `prototype/<ticket>`:

```text
acme-shop-10 claimed (prototype): "Prototype: A theme picker in the header" in Spec acme-shop-8; worktree /Users/you/src/acme-shop.worktrees/acme-shop-10; resume: claude --resume …
```

Its first prompt tells the session to commit on that branch only, with no push and no PR, to record where the Prototype is in a `prototype` Tag (`prototype/acme-shop-10@1a2b3c4 /Users/you/src/acme-shop.worktrees/acme-shop-10`), to open its first Round with a "Try it" Question, and to finish with no branch. The Ticket panel shows the Tag as *Prototype at `/Users/you/src/acme-shop.worktrees/acme-shop-10` on `prototype/acme-shop-10` (`1a2b3c4`)*, so you can try it in that worktree while you answer.

Because it finishes with no branch, accepting a Prototype makes it Done straight away, whatever the Landing Method: nothing is merged, pushed or landed. The worktree stays through Needs Answer and In Review, and the Factory removes it once the Prototype is Done or Cancelled. It never deletes the `prototype/` branch, so `git switch prototype/acme-shop-10` brings the Prototype back.

The Factory leaves a Map's research Ticket (`wayfinder:research`) unclaimed for now, and claims the Tickets after it. A Ticket with any of these Labels is never run as an implementation Ticket.

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

Switching from Branch to Merge merges, on the next poll, every Accepted Ticket you haven't merged yet. Switching from Merge to Branch leaves every Accepted Ticket the Factory hasn't merged yet for you.

Under either method, you can still merge an In Review Ticket's branch yourself, and the Factory lands it.

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

The Factory never pulls, rebases or force-pushes. When the push fails, say because the remote is unreachable or has moved ahead of your base, the Ticket stays Accepted, with the base already merged, and the Factory comments once, quoting git:

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
