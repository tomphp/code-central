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
import { HttpDriver, SESSION, worldConnection } from "./driver";
import type { CIRunView, SessionEvent, State, UsageResponse } from "./types";

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

// Work is the branch and head commit an agent finishes with.
export interface Work {
  branch: string;
  commit: string;
}

// CIOutcome is how a pushed commit's CI run stands.
export type CIOutcome = "running" | "passed" | "failed";

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

export class Scenario extends HttpDriver {
  // specOf is each Project's default Spec, once made.
  private readonly specOf = new Map<string, string>();
  // claims are each Ticket's live Claim ID, for the agent's next write.
  private readonly claims = new Map<string, string>();

  // project makes a Project, as `code-central init` does.
  async project(key: string, displayName = key) {
    await this.agent("/api/projects/create_project", { key, display_name: displayName });
  }

  // spec makes a Spec and returns its ID.
  async spec(key: string, s: { title?: string; body?: string; labels?: string[] } = {}): Promise<string> {
    return (await this.agent(`${this.projectPath(key)}/create_spec`, { title: s.title ?? "A Spec", body: s.body ?? "", ...(s.labels && { labels: s.labels }) })).id;
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

  // claimFor claims a Ready Ticket as an agent in session, or returns the
  // Claim it already has, and returns its Claim ID.
  async claimFor(key: string, id: string, session = SESSION): Promise<string> {
    let claim = this.claims.get(id);
    if (!claim) {
      claim = await this.claimed(key, id, session);
      this.claims.set(id, claim);
    }
    return claim;
  }

  // askRound has the agent post a Round of questions, claiming the Ticket
  // first if it must, and returns the Round's number.
  async askRound(key: string, id: string, questions: QuestionSeed[] = QUESTIONS): Promise<number> {
    await this.agent(`${this.ticketPath(key, id)}/post_round`, { claim_id: await this.claimFor(key, id), questions });
    return (await this.api.getTicket(key, id)).rounds.at(-1)!.number;
  }

  // answerRound has the human answer the latest Round's open Questions in
  // order, each with its answer or its recommendation, which completes
  // the Round.
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
  async fail(key: string, id: string, report = REPORT) {
    await this.agent(`${this.ticketPath(key, id)}/report_failure`, { claim_id: await this.claimFor(key, id), ...report });
    this.claims.delete(id);
  }

  // inReview has the agent finish the Ticket with work, claiming it first
  // if it must.
  async inReview(key: string, id: string, work: Work = WORK, summary = "Done the work") {
    await this.agent(`${this.ticketPath(key, id)}/finish`, { claim_id: await this.claimFor(key, id), summary, ...work });
    this.claims.delete(id);
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
    const conclusion = outcome === "passed" ? "success" : outcome === "failed" ? "failure" : undefined;
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

  // factorySession has the Factory claim the Ticket in session and post
  // its Session Events and its model responses' Usage, and returns the
  // Claim ID.
  async factorySession(key: string, id: string, s: { session: string; events?: SessionEvent[]; usage?: UsageResponse[] }): Promise<string> {
    const claim = await this.claimFor(key, id, s.session);
    if (s.events?.length) await this.postSessionEvents(s.session, s.events);
    if (s.usage?.length) await this.postUsage(s.session, s.usage);
    return claim;
  }

  private async defaultSpec(key: string): Promise<string> {
    let spec = this.specOf.get(key);
    if (!spec) {
      spec = await this.spec(key);
      this.specOf.set(key, spec);
    }
    return spec;
  }
}
