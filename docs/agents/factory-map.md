# The Factory's landing files and `factorytest`

A map for lookups one grep can't answer. Read it before opening `factory/land.go` (about 1000 lines): find the file and function you need, then read only that part. `go doc` doesn't show unexported methods, so grep `^func (l \*lander) <name>`.

## Landing files (`factory/`)

All landing code is methods of `*lander` (Spec code-central-934). The session code (`factory.go` and the rest) reaches it only through `lander.poll`, `claimingHeld` and `landerHooks`.

| File | What is in it |
|---|---|
| `lander.go` | The lander's type, `poll` (the per-poll order of everything below) and `claimingHeld`. Start here to see what runs when. |
| `land.go` | Routing and planning for Merge, Branch and Push: `landEach` switches on `domain.LandingPolicy.Route` (`RouteMerge`, `RouteSpecBranch`, `RoutePullRequest`, `RouteBranch`); then `planMerge`, `mergeAndLand`, `landPushed`, `landOnCI` (the CI gate), `pushBranch`, `detectHandMerges`. It also holds the "can't land yet" comments (`cantLandIn`, `cantPushIn`, `commentOnce`), the conflict send-back (`sendBack`) and the base advance (`advanceBase`, `setBaseTo`). |
| `pullrequest.go` | Pull Request landing for one Ticket: `landPullRequest`, `landMerged`, `detectMergedPullRequests`, and keeping the local base level with the Push remote's (`syncBase`, `reportBase`). |
| `pullrequests.go` | The same per Repository (Spec code-central-848): `landPullRequestIn`, `landMergedIn`, `landIn`, `detectMergedPullRequestsIn`. |
| `specbranch.go` | Land per Spec: `specBranch` (`spec/<id>`), merging Tickets into it (`mergeIntoSpecBranch`), pushing it, `landSpecs`/`landSpec`/`mergeSpec`, Spec conflicts, and sweeping finished Spec Branches. |
| `replay.go` | Rebase (ADR 0067): replaying a Ticket onto its Spec Branch (`planReplay`, `replay.apply`), how a replayed branch is told (`replayedTip`), rebasing a Spec Branch onto the base at landing (`rebaseSpecBranch`, through `movebase.go`'s `rebuildOnto`; `specbranch.go`'s `planSpec` and `moveRebasedSpecBranches` call and apply it), and the fork refs, `refs/code-central/fork/<id>` (`forkPoint`, `setForkPoint`, `moveForkPoint`, `deleteForkPoint`). |
| `base.go` | Per-Repository base state (`baseState`): `locateBases`, `held` (a diverged or missing base holds claims), and the MCP check. |
| `movebase.go` | Merge + Push when the local and remote bases each hold commits the other lacks: `moveBase`, `rebuildOnto` (which also rebases a Spec Branch onto the base under Rebase). |
| `repositories.go` | The `repository` and `repositories` types, `syncRepositories` and `cloneOf` (clone or adopt each Repository), `baseOf`, `inBaseOf`. |
| `changedrepositories.go` | The Repositories a Ticket changed (`changedIn`, `changedHead`) and the finish check on them. |
| `repo.go` | `gitRepo`: thin git wrappers (`mergeTree`, `commitTree`, `fastForward`, `push`, `fetch`, `worktrees`, ...). |

Avoid reading `land.go` whole. Read `pullrequest.go` and `specbranch.go` for those two paths.

## `factorytest` (`factory/factorytest/`)

`factorytest.New(t)` gives a `*Harness`: a real service, a Home Repository (`h.Repo`, branch `main`), a fake `claude` and a proxy. `h.Start()` runs the Factory and returns a `*Running`; `Settle`, `Poll`, `Tick`, `Stop` and `WantOutput` drive and check it. Scripts for the fake `claude` are `h.Script(ticket, steps...)`, with the steps in `script.go` (`Finish`, `FinishIn`, `CommitIn`, `Git`, `WriteFile`, `PostRound`, `Result`, ...).

**World builders**

- `h.ReadyTicket(title)`, `StageTicket`, `LabelledTicket`, `NewTicket`: Tickets in the shared Spec.
- `h.CommitOn(branch, file, content)`, then `h.Finished(id, branch, commit, repos...)`, then `h.Accept(id)`: an Accepted Ticket without running a session. `accepted` in `land_test.go` bundles the four.
- `h.SetLandingMethod(core.SetLandingMethodInput{...})`: the Landing Method, Push, remote and Land per Spec.
- `h.AddRemote(name)`: a bare remote on the Home, named `name`, with nothing pushed. It gives the Home no Clone URL: the Home's Clone URL is its `origin` URL as the Factory reports it at start. To give the Home one, run `h.Git("remote", "set-url", "origin", "git@github.com:acme/web.git")` before `h.Start()` (see `landlocalhome_test.go`). For a Code Host URL under Pull Request, use `pullRequestOn`.
- `h.AddRepository(name)`: a second Repository with its own bare remote **and a Clone URL** (`.CloneURL`), cloned beside the Home. `h.MakeRepository(name)` is the same without adding it to the Project. `h.AddLocalRepository(name)`: no Clone URL, local-only, never cloned. All return a `*Repository` with `CommitOn`, `Remote(name)` and `Worktree(ticket)`.

**Test-file helpers** (unexported, in `factory/*_test.go`, not in `factorytest`)

| Helper | File | Does |
|---|---|---|
| `accepted(h, title, file, content)` | `land_test.go` | An Accepted Ticket whose branch adds `file`. |
| `mergeAndPush(h, remote)` | `push_test.go` | Merge with Push to `remote`. |
| `pullRequestOn(h, remote)` | `pullrequest_test.go` | The Pull Request method; first reports a Code Host URL as the Push remote. |
| `wantPushed`, `wantPushedIn(t, h, repo, ticket)` | `pullrequest_test.go`, `pullrequests_test.go` | The Factory reported the branch pushed (in `repo`); returns the commit. |
| `trackCI(t, h)`, `watchCIIn(t, h, name, repo)` | `ci_test.go`, `landrepositories_test.go` | CI Tracking on for the Home / for a Repository. |
| `inTwo(h, title)`, `inAPI(h, title)` | `landrepositories_test.go` | A Ready Ticket whose session commits in the Home and `api` / in `api` alone. |
| `setPushRemote(t, h, name, remote)` | `landlocalhome_test.go` | Give a Repository its own Push remote. |

Pick by the world you need: a Ticket in the Home only (`accepted`), in several Repositories (`inTwo`), under Pull Request (`pullRequestOn`, `wantPushedIn`).
