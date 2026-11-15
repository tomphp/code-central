// The Focus Item as Focus Mode shows it, on the WASM Core: one story per
// Needs Me reason (all 17), the Question a Round's open Questions become,
// and each Landing Method variant that changes the options. Each world is
// built with the scenario builder, read back as the Inbox reads it, and
// handed to the Focus session, so the story shows the whole Focus Item
// with its Focus badge, Skip and Open in Inbox, not the bare panel. The
// api is Core's, since the panels act on it.
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { Api } from "../api/api";
import type { World } from "../api/core/core";
import { storyWorld } from "../api/core/story";
import { SESSION } from "../api/driver";
import type { Scenario } from "../api/scenario";
import type { LandingMethod } from "../api/types";
import { Loading } from "../controls/Empty";
import { loadInbox, type InboxData } from "../live";
import { NeedsMeProvider } from "../needsme/context";
import { needsMeOf } from "../needsme/model";
import { FocusItemView, inboxOf } from "./FocusItemView";
import { useFocusSession } from "./session";

const meta = { title: "Focus Items/FocusItemView" } satisfies Meta;
export default meta;
type Story = StoryObj<typeof meta>;

const MIN = 60_000;
const COMMIT = "abc1234567";
const RUN_URL = "https://github.com/acme/web/actions/runs/1";
const ISSUE = { integration: "github", id: "acme/web#7", url: "https://github.com/acme/web/issues/7" };
const QUESTIONS = [
  { title: "Which colour?", body: "Red or blue", recommended: "Blue" },
  { title: "Which size?", body: "S or M" },
];

interface Loaded {
  api: Api;
  now: Date;
  data: InboxData;
  // scope is the Ticket whose Focus Item shows; none shows the Project's own.
  scope?: string;
  // whole shows a Round as the one Focus Item Needs Me has for it, not as
  // its first open Question.
  whole: boolean;
}

// Shown is Focus Mode's layout over the Focus session's item, without the
// header: the item as Focus Mode shows it.
function Shown({ api, now, data, scope, whole }: Loaded) {
  const session = useFocusSession({ api, version: 0, project: "acme", scope });
  const item = session.item && whole ? { ...session.item, question: undefined } : session.item;
  return (
    <main aria-label="Focus Mode" className="focus">
      {item ? (
        <FocusItemView
          key={item.key}
          api={session.api}
          now={now}
          data={data}
          item={item}
          ticket={session.ticket}
          navigate={() => {}}
          skip={session.skip}
          inbox={inboxOf(item.row, "acme")}
        />
      ) : (
        <Loading />
      )}
    </main>
  );
}

// item is a story of the Focus Item of the Ticket that build makes in the
// Project "acme", returning its ID; without an ID it is the Project's own.
const item = (build: (s: Scenario, w: World) => Promise<string | undefined>, whole = false): Story => ({
  ...storyWorld(async (s, w): Promise<Loaded> => {
    await s.project("acme", "Acme");
    const scope = await build(s, w);
    return { api: w.api, now: w.now(), data: await loadInbox(w.api), scope, whole };
  }),
  render: (_, { loaded }) => {
    const l = loaded.world as Loaded;
    return (
      <NeedsMeProvider value={needsMeOf(l.data)}>
        <Shown {...l} />
      </NeedsMeProvider>
    );
  },
});

// setLanding has the human set acme's Landing Method, landing on each
// Ticket's own, and turn CI Tracking on.
async function setLanding(s: Scenario, method: LandingMethod, push: boolean) {
  await s.human(`${s.projectPath("acme")}/set_landing_method`, { method, push, remote: "origin", land_per_spec: false });
  await s.human(`${s.projectPath("acme")}/set_ci_tracking`, { tracking: true, provider: "github_actions", repository: "acme/web", ignored_workflows: [] });
}

// ciFailed is "Login page" Accepted under method with Push, its pushed
// commit's CI failed on one of three jobs.
async function ciFailed(s: Scenario, method: LandingMethod) {
  if (method === "pull_request") await s.reportPushRemote("acme", "git@github.com:acme/web.git");
  await setLanding(s, method, true);
  const id = await s.ticket("acme", { title: "Login page", state: "Accepted" });
  await s.reportPushedCommit("acme", id, COMMIT, id);
  if (method === "pull_request") await s.reportPullRequest("acme", { branch: id, number: 7, link: "https://github.com/acme/web/pull/7", state: "open" });
  await s.reportCIRuns("acme", [
    {
      id: 1,
      attempt: 1,
      workflow: "ci.yml",
      name: "CI",
      branch: "main",
      commit: COMMIT,
      status: "completed",
      conclusion: "failure",
      url: RUN_URL,
      jobs: [
        { name: "test", conclusion: "failure", url: `${RUN_URL}/job/1` },
        { name: "lint", conclusion: "success", url: `${RUN_URL}/job/2` },
        { name: "build", conclusion: "timed_out", url: `${RUN_URL}/job/3` },
      ],
      ticket: id,
    },
  ]);
  return id;
}

// accepted is an Accepted Ticket of a long-lived Spec, whose Tickets
// land one by one, each with a pull request under Pull Request.
async function accepted(s: Scenario) {
  const spec = await s.spec("acme", { labels: ["long-lived"] });
  return s.ticket("acme", { title: "Login form", state: "Accepted", spec });
}

// --- One story per Needs Me reason, in group order. ---

// A Round no Question of which is shown yet: the Focus Item Needs Me has
// for it, with the Round's panel.
export const NeedsAnswer = item(async (s) => {
  const id = await s.ticket("acme", { title: "Pick a colour" });
  await s.askRound("acme", id, QUESTIONS);
  return id;
}, true);

export const Answered = item((s) => s.ticket("acme", { title: "Pick a colour", state: "Answered" }));

// In Progress with no word from its session for 45 minutes.
export const SilentClaim = item(async (s, w) => {
  const id = await s.ticket("acme", { title: "Wire the API", state: "In Progress" });
  w.advance(45 * MIN);
  return id;
});

// In Progress for over an hour, its session busy all along.
export const LongClaim = item(async (s, w) => {
  const id = await s.ticket("acme", { title: "Wire the API", state: "In Progress" });
  w.advance(73 * MIN);
  await s.postSessionEvents(SESSION, [{ time: w.now().toISOString(), kind: "tool_call", tool: "Bash", tool_use_id: "tu-1", text: "Bash: make test" }]);
  w.advance(2 * MIN);
  return id;
});

export const Failed = item((s) => s.ticket("acme", { title: "Flaky build", state: "Failed" }));

export const InReview = item((s) => s.ticket("acme", { title: "Login page", state: "In Review" }));

// CI failed under Merge + Push, which offers Land anyway.
export const CIFailed = item((s) => ciFailed(s, "merge"));

export const CICancelled = item(async (s) => {
  const id = await s.ticket("acme", { title: "Login page", state: "Accepted" });
  await s.pushed("acme", id);
  await s.ci("acme", id, "cancelled");
  return id;
});

// A Project's item: both commits, and the fix is the human's by hand.
export const BaseDiverged = item(async (s) => {
  await s.base("acme", "diverged");
  return undefined;
});

// A Project's item: the branch that exists neither locally nor remotely.
export const BaseMissing = item(async (s) => {
  await s.base("acme", "missing");
  return undefined;
});

export const PullRequestClosed = item(async (s) => {
  const id = await accepted(s);
  await s.pullRequest("acme", id, "closed");
  return id;
});

// To merge under Branch: the human merges the branch into the base.
export const ToMerge = item(async (s) => {
  await s.human(`${s.projectPath("acme")}/set_landing_method`, { method: "branch", push: false, remote: "origin", land_per_spec: false });
  return s.ticket("acme", { title: "Login page", state: "Accepted" });
});

// No word of a pushed commit's CI 12 minutes after its push.
export const CIUnavailable = item(async (s, w) => {
  const id = await s.ticket("acme", { title: "Login page", state: "Accepted" });
  await s.pushed("acme", id);
  await s.reportCIWatcherStatus("acme", { state: "watching" });
  w.advance(12 * MIN);
  return id;
});

export const AcceptedNotLanded = item(async (s, w) => {
  await s.human(`${s.projectPath("acme")}/set_landing_method`, { method: "merge", push: false, remote: "origin", land_per_spec: false });
  const id = await s.ticket("acme", { title: "Login page", state: "Accepted" });
  w.advance(25 * MIN);
  return id;
});

export const BlockedByCancelled = item(async (s) => {
  const cancelled = await s.ticket("acme", { title: "Old plan", state: "Cancelled" });
  const blocked = await s.ticket("acme", { title: "Build on it" });
  await s.addBlocker("acme", blocked, cancelled);
  return blocked;
});

// An Incoming Issue awaiting triage.
export const NewIssue = item(async (s, w) => {
  const on = await w.api.setGitHubIssues("acme", { on: true, repository: "acme/web", label: "", new_issues_need_me: true });
  return s.incomingIssue("acme", on.github_issues.spec!, "Login is broken", ISSUE);
});

export const ClosedExternally = item((s) => s.linkedTicket("acme", { title: "Login is broken", link: { ...ISSUE, external_state: "closed" } }));

// --- A Round's Question. ---

// One open Question of a Round of two, with its answer form.
export const Question = item(async (s) => {
  const id = await s.ticket("acme", { title: "Pick a colour" });
  await s.askRound("acme", id, QUESTIONS);
  return id;
});

// --- Landing Method variants that change the options. ---

// CI failed under Branch + Push: no Land anyway, since the human's merge decides.
export const CIFailedBranch = item((s) => ciFailed(s, "branch"));

// CI failed under Pull Request: no Land anyway; Send back to fix and New
// fix Ticket stay, and the pull request is the merge that decides.
export const CIFailedPullRequest = item((s) => ciFailed(s, "pull_request"));

// CI failed for a Spec landing together: the item lists every Ticket
// waiting on the Spec's pushed commit, and offers no Send back to fix.
export const CIFailedLandPerSpec = item(async (s) => {
  await s.landOn("acme", "merge", false);
  const spec = await s.requirement("acme", "Export a Spec\nAs Markdown.");
  const ids = [
    await s.ticket("acme", { title: "Exporter", state: "Accepted", spec }),
    await s.ticket("acme", { title: "Export button", state: "Accepted", spec }),
    await s.ticket("acme", { title: "Export docs", state: "Accepted", spec }),
  ];
  for (const id of ids) {
    await s.specMerged("acme", id);
    await s.pushed("acme", id, COMMIT);
  }
  await s.ci("acme", ids[0], "failed", COMMIT);
  return ids[0];
});

// To merge under Pull Request: the pull request's link, here conflicting.
export const ToMergePullRequest = item(async (s) => {
  const id = await accepted(s);
  await s.pullRequest("acme", id, "conflicting");
  return id;
});
