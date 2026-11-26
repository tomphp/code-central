# The Factory runs on one injected clock and tells tests when it has settled

The Factory's tests were the least stable in the repository, and each fix patched the same gap: the rig could see that a poll had started, but not that its consequences had finished. So tests read past the Factory's interface to guess where it was. They counted `claim: ` lines on stdout, polled Code Central for up to two minutes, listed the fake claude's calls and ran `git rev-parse` in worktrees. Time also ran on two clocks: only the poll wait and the outage backoff were injected, so a test of a retry pause or a kill grace depended on how loaded the machine was.

Now every timer of the Factory and of the Watcher supervisor's stop limit waits on one injected `clock.Clock`: the poll wait, the claim, wake and Chat retry pauses, the Session Event and token usage batches, the kill grace, the outage backoff, and the timestamps the Factory writes. And the Factory tells an optional test hook, `Env.Work`, of its work in flight: each piece busy (a poll, a post, git work, handling a turn's exit) or resting (a timer, a long-poll, a turn's claude running, the wait for a signal), with each poll wait's deadline. Work that follows other work starts before the other ends, so no moment between them looks settled. The binary gives no hook, and Factory Status doesn't change.

The test rig, `factorytest`, builds one call on the hook, `Running.Settle`: it returns once nothing the Factory is doing will change anything without an outside event, every turn whose claude runs is parked in the fake's `Wait` step, and no answer is on its way through the rig's proxy. On a timeout it names what is still in flight. `Tick` moves the fake clock through each timer due before the next poll, settling after each, and then to the poll; `Poll` is a Tick and a Settle. A test never reads the Factory's output to learn where it is.

## Considered Options

- **A poll hook beside the clock**, telling the rig when a poll starts. Rejected: it is a second mechanism next to the clock, and it says when a poll starts, not when its consequences have finished.
- **Real-time last-resort kills and retry pauses.** Rejected: they made the signal and lost-claim tests depend on load, which is how they flaked.
- **A settled stream Factory Status could show.** Rejected: "busy" in Factory Status would be a product feature with no user; settled is a term of the test rig only.
- **Stdout counting and polling waits.** Rejected: they are the cause of the flakes, each a guess about timing.

## Consequences

- Deadlines that bound a real network or process call stay on real time: `postTimeout`, `EventPostTimeout`, `helpTimeout`, `checkLimit` and `os/exec`'s `WaitDelay`. No test moves the clock while a call hangs, so on a fake clock a hung call would never end.
- A turn's claude printing is an outside event, so what it printed before parking may reach a later batch post: a test asserting Session Events or token usage polls until they are posted.
- A test whose turn runs a claude other than the scripted fake can't settle while it runs, since it never parks in a `Wait` step: it waits on the outside effect it needs, as a test does for Code Central state no Factory work changes, such as a Chat reply.
- New Factory work has to report itself to the hook, and hand its busy work over at each goroutine it wakes; a gap shows up as a test that settles too early.
