// The scenario builder (Spec code-central-580): the Driver plus helpers
// that build whole flows, for component tests and stories alike. Each
// helper reaches its state through the operations agents, integrations,
// the Factory and the human make, never by writing it, so every derived
// field (Needs Me, CI result, Model Choice, Land per Spec, State) is the
// one Core works out.
//
//   const w = await newWorld();
//   const s = scenario(w);
//   await s.project("acme", "Acme");
//   const id = await s.ticket("acme", { title: "Pick a colour", state: "Needs Answer" });
//   render(<App api={w.api} now={w.now} />);
//
// IDs are what the writes returned (Specs and Tickets share one counter),
// and Question numbers are Core's.
import type { World } from "./core/core";
import { HttpDriver, worldConnection } from "./driver";
import type { CIRunView, ExternalState, IssueWatcherReport, PullRequestState, PullRequestWatcherReport, SessionEvent, State, UsageResponse } from "./types";

export interface QuestionSeed {
  title: string;
  body: string;
  recommended?: string;
}

export interface TicketSeed {
  title: string;
  // spec is the Spec the Ticket goes under: the Project's own "A Spec",
  // made the first time, unless named.
  spec?: string;
  // state is where the Ticket ends up, Ready unless named; see ticket.
  state?: State;
  description?: string;
  labels?: string[];
}

// LinkSeed is the external item an Integration links a Ticket or Spec to.
export interface LinkSeed {
  integration: string;
  id: string;
  url?: string;
  external_state?: ExternalState;
}

// Work is the branch and head commit an agent finishes with.
export interface Work {
  branch: string;
  commit: string;
}

// CIOutcome is how a pushed commit's CI run stands.
export type CIOutcome = "running" | "passed" | "failed" | "cancelled";

// scenario builds on a WASM Core world.
export function scenario(w: World): Scenario {
  return new Scenario(worldConnection(w));
}

export const QUESTIONS: QuestionSeed[] = [{ title: "Which colour?", body: "Red or blue", recommended: "Blue" }];
export const WORK: Work = { branch: "impl", commit: "abc123" };
export const REPORT = {
  what_broke: "Tests fail on CI\nlogin_test.go times out",
  why: "The fixture server never starts",
  what_to_fix: "Start the fixture in TestMain",
  where_work_stands: "Branch impl, two commits, tests red",
};

// The helpers share the Driver's Claims and Projects' Specs, so they and
// its own operations mix on one Ticket.
export class Scenario extends HttpDriver {
  // project makes a Project, as `code-central init` does.
  async project(key: string, displayName = key) {
    await this.agent("/api/projects/create_project", { key, display_name: displayName });
  }

  // spec makes a Spec and returns its ID.
  async spec(key: string, s: { title?: string; body?: string; labels?: string[] } = {}): Promise<string> {
    const { id } = await this.agent(`${this.projectPath(key)}/create_spec`, { title: s.title ?? "A Spec", body: s.body ?? "" });
    if (s.labels?.length) await this.api.addLabels(key, id, s.labels);
    return id;
  }

  // ticket makes a Ticket and takes it to its State the way it gets
  // there, returning its ID: In Progress is claimed; Needs Answer has a
  // Round of QUESTIONS asked and Answered has it answered; Failed has
  // REPORT; In Review is finished with WORK, Accepted accepted and Done
  // landed; Cancelled is cancelled by the human.
  async ticket(key: string, t: TicketSeed): Promise<string> {
    const spec = t.spec ?? (await this.defaultSpec(key));
    const state = t.state ?? "Ready";
    const { id } = await this.agent(`${this.projectPath(key)}/create_ticket`, {
      spec,
      title: t.title,
      state: state === "Draft" ? "Draft" : "Ready",
      ...(t.description && { description: t.description }),
      ...(t.labels && { labels: t.labels }),
    });
    switch (state) {
      case "Draft":
      case "Ready":
        break;
      case "In Progress":
        await this.claimFor(key, id);
        break;
      case "Needs Answer":
        await this.askRound(key, id);
        break;
      case "Answered":
        await this.askRound(key, id);
        await this.answerRound(key, id);
        break;
      case "Failed":
        await this.fail(key, id);
        break;
      case "In Review":
        await this.inReview(key, id);
        break;
      case "Accepted":
        await this.accepted(key, id);
        break;
      case "Done":
        await this.accepted(key, id);
        await this.human(`${this.ticketPath(key, id)}/land`, { commit: WORK.commit });
        break;
      case "Cancelled":
        await this.human(`${this.ticketPath(key, id)}/cancel`, {});
        break;
    }
    return id;
  }

  // linkedTicket has an Integration make a Ticket linked to an external
  // item, Draft unless state says Ready, and returns its ID.
  async linkedTicket(key: string, t: { title: string; link: LinkSeed; spec?: string; state?: "Draft" | "Ready" }): Promise<string> {
    const spec = t.spec ?? (await this.defaultSpec(key));
    return (await this.integration(`${this.projectPath(key)}/integration/create_linked_ticket`, { spec, title: t.title, ...(t.state && { state: t.state }), link: t.link })).id;
  }

  // linkedSpec has an Integration make a Spec linked to an external item,
  // and returns its ID.
  async linkedSpec(key: string, title: string, link: LinkSeed): Promise<string> {
    return (await this.integration(`${this.projectPath(key)}/integration/create_linked_spec`, { title, link })).id;
  }

  // modelChoice has the Factory report the model and effort its claude
  // started the Ticket's latest turn with, claiming the Ticket first if it must.
  async modelChoice(key: string, id: string, c: { model?: string; effort?: string }) {
    await this.agent(`${this.ticketPath(key, id)}/report_model_choice`, { claim_id: await this.claimFor(key, id), ...c });
  }

  // askRound has the agent post a Round of questions, claiming the Ticket
  // first if it must, and returns the Round's number.
  askRound(key: string, id: string, questions: QuestionSeed[] = QUESTIONS): Promise<number> {
    return this.postRound(key, id, questions);
  }

  // answerRound has the human answer the latest Round's open Questions in
  // order, each with its answer, else its recommendation, else "Yes",
  // which completes the Round.
  async answerRound(key: string, id: string, answers: string[] = []) {
    const open = (await this.api.getTicket(key, id)).rounds.at(-1)!.questions.filter((q) => q.status === "open");
    for (const [i, q] of open.entries()) {
      await this.human(`${this.ticketPath(key, id)}/answer`, { question: q.number, text: answers[i] ?? q.recommended ?? "Yes" });
    }
  }

  // pickUpAnswers has the agent pick up an Answered Ticket's answers,
  // back to In Progress.
  async pickUpAnswers(key: string, id: string) {
    await this.agent(`${this.ticketPath(key, id)}/pick_up_answers`, { claim_id: await this.claimFor(key, id) });
  }

  // fail has the agent report a failure, claiming the Ticket first if it must.
  fail(key: string, id: string, report = REPORT) {
    return this.reportFailure(key, id, report);
  }

  // inReview has the agent finish the Ticket with work, claiming it first
  // if it must.
  inReview(key: string, id: string, work: Work = WORK, summary = "Done the work") {
    return this.finish(key, id, summary, work);
  }

  // accepted takes the Ticket In Review with work and has the human accept it.
  async accepted(key: string, id: string, work: Work = WORK) {
    await this.inReview(key, id, work);
    await this.human(`${this.ticketPath(key, id)}/accept`, {});
  }

  // trackCI has the human set Merge with Push to origin and turn CI
  // Tracking on for repository, under which a pushed commit has CI.
  async trackCI(key: string, repository = "acme/web") {
    await this.human(`${this.projectPath(key)}/set_landing_method`, { method: "merge", push: true, remote: "origin" });
    await this.human(`${this.projectPath(key)}/set_ci_tracking`, { tracking: true, provider: "github_actions", repository, ignored_workflows: [] });
  }

  // pushed has the Factory report it pushed an Accepted Ticket's commit,
  // tracking CI first (trackCI) unless it is: the commit's CI is running
  // until runs report.
  async pushed(key: string, id: string, commit = WORK.commit) {
    const p = (await this.api.listProjects()).find((p) => p.key === key);
    if (!p?.ci.tracking || !p.landing.push) await this.trackCI(key);
    await this.reportPushedCommit(key, id, commit);
  }

  // ci has the CI Watcher report one run of the commit's workflow ci.yml
  // (run number run) as outcome says.
  async ci(key: string, id: string, outcome: CIOutcome, commit = WORK.commit, run = 1) {
    const conclusion = { passed: "success", failed: "failure", cancelled: "cancelled", running: undefined }[outcome] as CIRunView["conclusion"];
    const r: Omit<CIRunView, "ignored"> = {
      id: run,
      attempt: 1,
      workflow: "ci.yml",
      name: "CI",
      branch: "main",
      commit,
      status: conclusion ? "completed" : "in_progress",
      ...(conclusion && { conclusion }),
      url: `https://github.com/acme/web/actions/runs/${run}`,
      jobs: [{ name: "test", ...(conclusion && { conclusion }), url: `https://github.com/acme/web/actions/runs/${run}/job/1` }],
      ticket: id,
    };
    await this.reportCIRuns(key, [r]);
  }

  // requirement has the human submit text as a Requirement and each of
  // its Stages be worked, accepted and landed, so it stands in Build. It
  // returns the Requirement's Spec ID.
  async requirement(key: string, text: string): Promise<string> {
    const r = await this.api.submitRequirement(key, text);
    for (const t of r.stages) {
      const claim_id = await this.claimFor(key, t.id);
      // Only the spec Stage takes a recommendation.
      const spec = (await this.api.getTicket(key, t.id)).labels.includes("stage:spec");
      await this.agent(`${this.ticketPath(key, t.id)}/finish`, { claim_id, summary: "Done the Stage", ...(spec && { recommendation: "implement" }), ...WORK });
      await this.human(`${this.ticketPath(key, t.id)}/accept`, {});
      await this.human(`${this.ticketPath(key, t.id)}/land`, { commit: WORK.commit });
    }
    return r.spec.id;
  }

  // specMerged has the Factory report it merged an Accepted Spec-landed
  // Ticket's work into its Spec Branch at commit.
  async specMerged(key: string, id: string, commit = WORK.commit) {
    await this.agent(`${this.ticketPath(key, id)}/report_spec_merge`, { ticket: id, commit });
  }

  // landOn has the human set the Project's Landing Method.
  async landOn(key: string, method: "merge" | "pull_request", push = method === "merge") {
    await this.human(`${this.projectPath(key)}/set_landing_method`, { method, push, remote: "origin" });
  }

  // base has a Factory come online for the Project and report its base
  // as state says. A diverged base shows only under Merge + Push or Pull
  // Request, so landOn sets Merge + Push first unless the Project has one.
  async base(key: string, state: "current" | "diverged" | "missing", commits: { local: string; remote: string } = { local: "aaa1111222", remote: "bbb2222333" }) {
    const p = (await this.api.listProjects()).find((p) => p.key === key);
    if (state === "diverged" && !p?.landing.push && p?.landing.method !== "pull_request") await this.landOn(key, "merge");
    await this.reportHeartbeat(key, { factory: "code-central-factory", host: "laptop", state: "online", interval_seconds: 30 });
    await this.agent(`${this.projectPath(key)}/report_base`, { state, ...commits });
  }

  // pullRequest takes an Accepted Ticket to the Pull Request Landing
  // Method with its branch pushed and its pull request reported in state.
  async pullRequest(key: string, id: string, state: PullRequestState = "open", number = 7) {
    await this.reportPushRemote(key, "git@github.com:acme/web.git");
    await this.landOn(key, "pull_request");
    await this.reportPushedCommit(key, id, "abc123", id);
    await this.reportPullRequest(key, {
      branch: id,
      number,
      link: `https://github.com/acme/web/pull/${number}`,
      state,
      ...(state === "merged" && { merge_commit: "def456" }),
    });
  }

  // factorySession has the Factory claim the Ticket in session and post
  // its Session Events and its model responses' Usage, and returns the
  // Claim ID.
  async factorySession(key: string, id: string, s: { session: string; events?: SessionEvent[]; usage?: UsageResponse[] }): Promise<string> {
    const claim = await this.claimFor(key, id, s.session);
    if (s.events?.length) await this.postSessionEvents(s.session, s.events);
    if (s.usage?.length) await this.postUsage(s.session, s.usage);
    return claim;
  }

  // chatReply has the Factory holding a Conversation (the one a turn was
  // taken for) post items of the running reply and, when it says so, how
  // the reply ended, with the session's Resume Command on the first post.
  async chatReply(
    key: string,
    conversation: string,
    items: { kind?: string; text: string; tool?: string; tool_use_id?: string; is_error?: boolean }[],
    end: { outcome?: "done" | "stopped" | "failed"; error?: string; resume_command?: string } = {},
  ) {
    await this.agent(`${this.projectPath(key)}/chat/events`, {
      conversation,
      factory: "code-central-factory",
      host: "laptop",
      session_id: end.resume_command ? "session-1" : "",
      resume_command: end.resume_command ?? "",
      items: items.map((i) => ({ time: "2026-11-07T09:00:00Z", kind: "text", ...i })),
      outcome: end.outcome ?? "",
      error: end.error ?? "",
    });
  }

  // archive has the human put the Project away.
  async archive(key: string) {
    await this.api.archiveProject(key);
  }

  // reportIssueWatcher and reportPullRequestWatcher have the Watchers
  // report their status, as the Issue Watcher and the Pull Request
  // Watcher do on each poll.
  async reportIssueWatcher(key: string, status: IssueWatcherReport) {
    await this.integration(`${this.projectPath(key)}/integration/report_issue_watcher_status`, status);
  }

  async reportPullRequestWatcher(key: string, status: PullRequestWatcherReport) {
    await this.agent(`${this.projectPath(key)}/report_pull_request_watcher_status`, status);
  }

  // modelIDs has a Factory's session on a Ticket post Usage naming each
  // model, which is how Core collects a Project's Reported model IDs.
  async modelIDs(key: string, ids: string[], session = "model-ids") {
    const id = await this.ticket(key, { title: "Uses models" });
    await this.factorySession(key, id, {
      session,
      usage: ids.map((model) => ({ time: "2026-11-07T09:00:00Z", kind: "implementation", model, input_tokens: 1, cache_write_tokens: 0, cache_read_tokens: 0, output_tokens: 1 })),
    });
  }

  // defaultSpec is the Project's Spec for Tickets that name none: the one
  // createProject made, else "A Spec", made the first time.
  private async defaultSpec(key: string): Promise<string> {
    let spec = this.specs.get(key);
    if (!spec) {
      spec = await this.spec(key);
      this.specs.set(key, spec);
    }
    return spec;
  }
}
