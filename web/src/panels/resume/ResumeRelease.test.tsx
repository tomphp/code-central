// The Resume-then-Release panel, for an Answered Ticket and a Silent or
// Long Claim, on the WASM Core through the real App. On a Silent or Long
// Claim, release reads "Stop".
import { render, screen, within, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../../App";
import { newWorld, type World } from "../../api/core/core";
import { scenario, type Scenario } from "../../api/scenario";
import { panel as resumePanel } from "./panel";

const MINUTE = 60_000;

// claimAs has an agent claim the Ticket in session, with where it runs.
async function claimAs(s: Scenario, id: string, claim: { session_id?: string; working_dir?: string; resume_command?: string }) {
  await s.agent(`${s.projectPath("acme")}/claim`, { ticket: id, ...claim });
}

// silentWorld has a Ticket In Progress whose Claim has been quiet for 45
// minutes.
async function silentWorld(session_id?: string, over: { working_dir?: string; resume_command?: string } = {}) {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  await s.project("spare", "Spare"); // a second active Project, so All Projects stays offered
  const id = await s.ticket("acme", { title: "Wire the API" });
  await claimAs(s, id, { session_id, ...over });
  w.advance(45 * MINUTE);
  return { w, s, id };
}

// longWorld has a Ticket In Progress since 75 minutes ago whose session
// has been busy since.
async function longWorld() {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  await s.project("spare", "Spare"); // a second active Project, so All Projects stays offered
  const id = await s.ticket("acme", { title: "Wire the API" });
  await claimAs(s, id, { session_id: "sess-42" });
  w.advance(73 * MINUTE);
  await s.postSessionEvents("sess-42", [{ time: w.now().toISOString(), kind: "tool_call", tool: "Bash", tool_use_id: "tu-1", text: "Bash: make test" }]);
  w.advance(2 * MINUTE);
  return { w, s, id };
}

// answeredWorld has a Ticket Answered three hours ago, no session having
// picked it up: its Round 2 posted in session sess-round, whose Claim has
// since recorded sess-claim, or none if claimSession is false.
async function answeredWorld(claimSession = true) {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  await s.project("spare", "Spare"); // a second active Project, so All Projects stays offered
  const id = await s.ticket("acme", { title: "Pick a colour" });
  await s.claim("acme", id, "sess-round");
  await s.askRound("acme", id, [{ title: "Old question", body: "Asked before" }]);
  await s.answerRound("acme", id, ["Yes"]);
  await s.pickUpAnswers("acme", id);
  await s.askRound("acme", id, [
    { title: "Which colour?", body: "Red or blue" },
    { title: "Which size?", body: "S or M" },
    { title: "Rounded corners?", body: "Or square" },
  ]);
  const [, size] = (await w.api.getTicket("acme", id)).rounds[1].questions;
  await s.human(`${s.ticketPath("acme", id)}/answer`, { question: size.number - 1, text: "Blue" });
  await s.human(`${s.ticketPath("acme", id)}/dismiss_question`, { question: size.number });
  await s.human(`${s.ticketPath("acme", id)}/answer`, { question: size.number + 1, text: "**yes**, `4px`" });
  await s.agent(`${s.ticketPath("acme", id)}/set_session`, { claim_id: await s.claimFor("acme", id), session_id: claimSession ? "sess-claim" : "" });
  w.advance(3 * 60 * MINUTE);
  return { w, s, id };
}

function renderApp(w: World, id: string) {
  window.history.replaceState(null, "", `/?view=inbox&ticket=${id}`);
  return render(<App api={w.api} now={w.now} />);
}

const panel = () => screen.findByRole("region", { name: "Resume or release" });

describe("the Resume-then-Release panel", () => {
  it("shows a Silent Claim's quiet time, then a copyable resume command for its session", async () => {
    const user = userEvent.setup();
    const { w, id } = await silentWorld("sess-42");
    renderApp(w, id);
    const p = await panel();
    expect(p).toHaveTextContent("Quiet for 45m");
    expect(within(p).getByText("claude --resume sess-42")).toBeInTheDocument();

    await user.click(within(p).getByRole("button", { name: "Copy" }));
    expect(await navigator.clipboard.readText()).toBe("claude --resume sess-42");
    expect(within(p).getByRole("button", { name: "Copied" })).toBeInTheDocument();
  });

  it("says when no session ID is recorded, and offers no resume command", async () => {
    const { w, id } = await silentWorld(undefined);
    renderApp(w, id);
    const p = await panel();
    expect(p).toHaveTextContent("No session ID is recorded");
    expect(within(p).queryByText(/claude --resume/)).toBeNull();
    expect(within(p).queryByRole("button", { name: "Copy" })).toBeNull();
  });

  it("shows the Claim's Resume Command from its working directory", async () => {
    const user = userEvent.setup();
    const { w, id } = await silentWorld("sess-42", { working_dir: "/work/acme-1", resume_command: "codex resume sess-42" });
    renderApp(w, id);
    const p = await panel();
    expect(within(p).getByText("cd /work/acme-1 && codex resume sess-42")).toBeInTheDocument();
    await user.click(within(p).getByRole("button", { name: "Copy" }));
    expect(await navigator.clipboard.readText()).toBe("cd /work/acme-1 && codex resume sess-42");
  });

  it("leaves out the cd when no working directory is recorded", async () => {
    const { w, id } = await silentWorld(undefined, { resume_command: "codex resume x" });
    renderApp(w, id);
    const p = await panel();
    expect(within(p).getByText("codex resume x")).toBeInTheDocument();
    expect(p).not.toHaveTextContent("No session ID is recorded");
  });

  it("falls back to claude --resume from the working directory when no command is set", async () => {
    const { w, id } = await silentWorld("sess-42", { working_dir: "/work/acme-1" });
    renderApp(w, id);
    expect(within(await panel()).getByText("cd /work/acme-1 && claude --resume sess-42")).toBeInTheDocument();
  });

  it("quotes a working directory the shell would split", async () => {
    const { w, id } = await silentWorld("sess-42", { working_dir: "/work/my repo", resume_command: "agent resume" });
    renderApp(w, id);
    expect(within(await panel()).getByText("cd '/work/my repo' && agent resume")).toBeInTheDocument();
  });

  it("on an Answered Ticket shows how long it has waited and the Round's answers, collapsed", async () => {
    const { w, id } = await answeredWorld();
    renderApp(w, id);
    const p = await panel();
    expect(p).toHaveTextContent("Answered 3h ago, no session has picked it up");
    // The live Claim's session wins over the Round's: set_session updates it.
    expect(within(p).getByText("claude --resume sess-claim")).toBeInTheDocument();

    const answers = within(p).getByText("Round 2 answers").closest("details")!;
    expect(answers).not.toHaveAttribute("open");
    expect(within(answers).getByText("Which colour?")).not.toBeVisible();
    expect(answers).toHaveTextContent("Which colour?");
    expect(answers).toHaveTextContent("Blue");
    expect(answers).toHaveTextContent("Which size?");
    expect(answers).toHaveTextContent("Dismissed");
    // Answers are what the human typed, so they stay plain text.
    expect(answers).toHaveTextContent("**yes**, `4px`");
  });

  it("falls back to the latest Round's session when the Claim has none", async () => {
    const { w, id } = await answeredWorld(false);
    renderApp(w, id);
    expect(within(await panel()).getByText("claude --resume sess-round")).toBeInTheDocument();
  });

  it("shows a Long Claim in Needs Me with how long since it last moved, its resume command, and Stop", async () => {
    const user = userEvent.setup();
    const { w, id } = await longWorld();
    renderApp(w, id);
    const list = screen.getByRole("main", { name: "Tickets" });
    expect(await within(list).findByRole("link", { name: new RegExp(id) })).toHaveTextContent("No move for 1h");
    expect(within(list).getByRole("heading", { level: 3 })).toHaveTextContent("Long Claim");

    const p = await panel();
    expect(p).toHaveTextContent("Active, but no move for 1h.");
    expect(within(p).getByText("claude --resume sess-42")).toBeInTheDocument();
    expect(within(p).queryByRole("button", { name: "Release…" })).toBeNull();

    await user.click(within(p).getByRole("button", { name: "Stop…" }));
    expect(within(p).getByRole("alert")).toHaveTextContent("a live session's next write will be rejected");
    await user.click(within(p).getByRole("button", { name: "Stop Claim" }));
    await waitFor(() => expect(w.calls).toContain("release"));
    expect((await w.api.getTicket("acme", id)).state).toBe("Ready");
  });

  it("labels release Stop on a Silent Claim and Release on an Answered Ticket", async () => {
    const silent = await silentWorld("sess-42");
    const view = renderApp(silent.w, silent.id);
    expect(within(await panel()).getByRole("button", { name: "Stop…" })).toBeInTheDocument();
    view.unmount();
    const answered = await answeredWorld();
    renderApp(answered.w, answered.id);
    expect(within(await panel()).getByRole("button", { name: "Release…" })).toBeInTheDocument();
  });

  it("warns before stopping a Silent Claim, then sends the note", async () => {
    const user = userEvent.setup();
    const { w, id } = await silentWorld("sess-42");
    renderApp(w, id);
    const p = await panel();
    expect(within(p).queryByRole("alert")).toBeNull();

    await user.click(within(p).getByRole("button", { name: "Stop…" }));
    const warning = within(p).getByRole("alert");
    expect(warning).toHaveTextContent("a live session's next write will be rejected");
    expect(warning).toHaveTextContent("can't be restored");
    expect(w.calls).not.toContain("release");

    await user.type(within(p).getByRole("textbox", { name: "Note (optional)" }), "Session died overnight");
    await user.click(within(p).getByRole("button", { name: "Stop Claim" }));

    await waitFor(() => expect(w.calls).toContain("release"));
    const t = await w.api.getTicket("acme", id);
    expect(t.state).toBe("Ready");
    expect(t.comments.at(-1)).toMatchObject({ actor: "human", body: "Session died overnight" });
    expect(t.claims[0]).toMatchObject({ ended_by: "release" });
  });

  it("releases without a note, and can be backed out of", async () => {
    const user = userEvent.setup();
    const { w, id } = await answeredWorld();
    const before = (await w.api.getTicket("acme", id)).comments;
    renderApp(w, id);
    const p = await panel();

    await user.click(within(p).getByRole("button", { name: "Release…" }));
    await user.click(within(p).getByRole("button", { name: "Keep the Claim" }));
    expect(within(p).queryByRole("alert")).toBeNull();

    await user.click(within(p).getByRole("button", { name: "Release…" }));
    await user.click(within(p).getByRole("button", { name: "Release Claim" }));
    await waitFor(() => expect(w.calls).toContain("release"));
    const t = await w.api.getTicket("acme", id);
    expect(t.state).toBe("Ready");
    expect(t.comments).toEqual(before);
  });

  it("shows the service's error if the release is refused", async () => {
    const user = userEvent.setup();
    const { w, s, id } = await silentWorld("sess-42");
    // The panel is of the Ticket as it was; someone else released it
    // meanwhile, and the notice hasn't arrived yet.
    const stale = await w.api.getTicket("acme", id);
    await s.human(`${s.ticketPath("acme", id)}/release`, {});
    render(<resumePanel.Component api={w.api} project="acme" ticket={stale} now={w.now()} />);
    const p = await panel();
    await user.click(within(p).getByRole("button", { name: "Stop…" }));
    await user.click(within(p).getByRole("button", { name: "Stop Claim" }));
    expect(await within(p).findByText(/release is not allowed/)).toBeInTheDocument();
    expect(within(p).getByRole("button", { name: "Stop Claim" })).toBeEnabled();
  });
});
