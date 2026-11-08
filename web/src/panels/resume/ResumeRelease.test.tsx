// The Resume-then-Release panel, for an Answered Ticket and a Silent or
// Long Claim, against FakeApi through the real App. On a Silent or Long
// Claim, release reads "Stop".
import { render, screen, within, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../../App";
import { FakeApi } from "../../api/fake";
import { ApiError, type ClaimView } from "../../api/types";

const minutesAgo = (fake: FakeApi, m: number) => new Date(fake.now.getTime() - m * 60_000).toISOString();

function claim(fake: FakeApi, session_id?: string, over: Partial<ClaimView> = {}): ClaimView {
  return { number: 1, session_id, started: minutesAgo(fake, 120), last_activity: minutesAgo(fake, 45), ...over };
}

function silentWorld(session_id?: string, over: Partial<ClaimView> = {}) {
  const fake = new FakeApi();
  fake.addProject("acme", "Acme");
  fake.addTicket("acme", {
    title: "Wire the API",
    state: "In Progress",
    claims: [claim(fake, session_id, over)],
    silent_since: minutesAgo(fake, 45),
  });
  fake.setNeedsMe([{ reason: "silent_claim", tickets: [{ id: "acme-1", since: minutesAgo(fake, 45) }] }]);
  return fake;
}

function longWorld() {
  const fake = new FakeApi();
  fake.addProject("acme", "Acme");
  fake.addTicket("acme", {
    title: "Wire the API",
    state: "In Progress",
    claims: [{ number: 1, session_id: "sess-42", started: minutesAgo(fake, 75), last_activity: minutesAgo(fake, 2) }],
    moves: [{ from: "Ready", to: "In Progress", actor: "agent", time: minutesAgo(fake, 75), claim: 1 }],
  });
  fake.setNeedsMe([{ reason: "long_claim", tickets: [{ id: "acme-1", since: minutesAgo(fake, 75) }] }]);
  return fake;
}

function answeredWorld() {
  const fake = new FakeApi();
  fake.addProject("acme", "Acme");
  fake.addTicket("acme", {
    title: "Pick a colour",
    state: "Answered",
    claims: [claim(fake, "sess-claim")],
    rounds: [
      {
        number: 2,
        claim: 1,
        session_id: "sess-round",
        posted: minutesAgo(fake, 200),
        questions: [
          { number: 1, title: "Which colour?", body: "Red or blue", status: "answered", answer: "Blue" },
          { number: 2, title: "Which size?", body: "S or M", status: "dismissed" },
        ],
      },
    ],
    moves: [
      { from: "In Progress", to: "Needs Answer", actor: "agent", time: minutesAgo(fake, 200), claim: 1 },
      { from: "Needs Answer", to: "Answered", actor: "human", time: minutesAgo(fake, 3 * 60) },
    ],
  });
  fake.setNeedsMe([{ reason: "answered", tickets: [{ id: "acme-1", since: minutesAgo(fake, 3 * 60) }] }]);
  return fake;
}

function renderApp(fake: FakeApi) {
  window.history.replaceState(null, "", "/?view=inbox&ticket=acme-1");
  return render(<App api={fake} now={() => fake.now} />);
}

const panel = () => screen.findByRole("region", { name: "Resume or release" });

describe("the Resume-then-Release panel", () => {
  it("shows a Silent Claim's quiet time, then a copyable resume command for its session", async () => {
    const user = userEvent.setup();
    renderApp(silentWorld("sess-42"));
    const p = await panel();
    expect(p).toHaveTextContent("Quiet for 45m");
    expect(within(p).getByText("claude --resume sess-42")).toBeInTheDocument();

    await user.click(within(p).getByRole("button", { name: "Copy" }));
    expect(await navigator.clipboard.readText()).toBe("claude --resume sess-42");
    expect(within(p).getByRole("button", { name: "Copied" })).toBeInTheDocument();
  });

  it("says when no session ID is recorded, and offers no resume command", async () => {
    renderApp(silentWorld(undefined));
    const p = await panel();
    expect(p).toHaveTextContent("No session ID is recorded");
    expect(within(p).queryByText(/claude --resume/)).toBeNull();
    expect(within(p).queryByRole("button", { name: "Copy" })).toBeNull();
  });

  it("shows the Claim's Resume Command from its working directory", async () => {
    const user = userEvent.setup();
    renderApp(silentWorld("sess-42", { working_dir: "/work/acme-1", resume_command: "codex resume sess-42" }));
    const p = await panel();
    expect(within(p).getByText("cd /work/acme-1 && codex resume sess-42")).toBeInTheDocument();
    await user.click(within(p).getByRole("button", { name: "Copy" }));
    expect(await navigator.clipboard.readText()).toBe("cd /work/acme-1 && codex resume sess-42");
  });

  it("leaves out the cd when no working directory is recorded", async () => {
    renderApp(silentWorld(undefined, { resume_command: "codex resume x" }));
    const p = await panel();
    expect(within(p).getByText("codex resume x")).toBeInTheDocument();
    expect(p).not.toHaveTextContent("No session ID is recorded");
  });

  it("falls back to claude --resume from the working directory when no command is set", async () => {
    renderApp(silentWorld("sess-42", { working_dir: "/work/acme-1" }));
    expect(within(await panel()).getByText("cd /work/acme-1 && claude --resume sess-42")).toBeInTheDocument();
  });

  it("quotes a working directory the shell would split", async () => {
    renderApp(silentWorld("sess-42", { working_dir: "/work/my repo", resume_command: "agent resume" }));
    expect(within(await panel()).getByText("cd '/work/my repo' && agent resume")).toBeInTheDocument();
  });

  it("on an Answered Ticket shows how long it has waited and the Round's answers, collapsed", async () => {
    renderApp(answeredWorld());
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
  });

  it("falls back to the latest Round's session when the Claim has none", async () => {
    const fake = answeredWorld();
    fake.updateTicket("acme-1", { claims: [claim(fake, undefined)] });
    renderApp(fake);
    expect(within(await panel()).getByText("claude --resume sess-round")).toBeInTheDocument();
  });

  it("shows a Long Claim in Needs Me with how long since it last moved, its resume command, and Stop", async () => {
    const user = userEvent.setup();
    const fake = longWorld();
    renderApp(fake);
    const list = screen.getByRole("main", { name: "Tickets" });
    expect(await within(list).findByRole("link", { name: /acme-1/ })).toHaveTextContent("No move for 1h");
    expect(within(list).getByRole("heading", { level: 3 })).toHaveTextContent("Long Claim");

    const p = await panel();
    expect(p).toHaveTextContent("Active, but no move for 1h.");
    expect(within(p).getByText("claude --resume sess-42")).toBeInTheDocument();
    expect(within(p).queryByRole("button", { name: "Release…" })).toBeNull();

    await user.click(within(p).getByRole("button", { name: "Stop…" }));
    expect(within(p).getByRole("alert")).toHaveTextContent("a live session's next write will be rejected");
    await user.click(within(p).getByRole("button", { name: "Stop Claim" }));
    await waitFor(() => expect(fake.calls).toContain("release"));
    expect((await fake.getTicket("acme", "acme-1")).state).toBe("Ready");
  });

  it("labels release Stop on a Silent Claim and Release on an Answered Ticket", async () => {
    const silent = renderApp(silentWorld("sess-42"));
    expect(within(await panel()).getByRole("button", { name: "Stop…" })).toBeInTheDocument();
    silent.unmount();
    renderApp(answeredWorld());
    expect(within(await panel()).getByRole("button", { name: "Release…" })).toBeInTheDocument();
  });

  it("warns before stopping a Silent Claim, then sends the note", async () => {
    const user = userEvent.setup();
    const fake = silentWorld("sess-42");
    renderApp(fake);
    const p = await panel();
    expect(within(p).queryByRole("alert")).toBeNull();

    await user.click(within(p).getByRole("button", { name: "Stop…" }));
    const warning = within(p).getByRole("alert");
    expect(warning).toHaveTextContent("a live session's next write will be rejected");
    expect(warning).toHaveTextContent("can't be restored");
    expect(fake.calls).not.toContain("release");

    await user.type(within(p).getByRole("textbox", { name: "Note (optional)" }), "Session died overnight");
    await user.click(within(p).getByRole("button", { name: "Stop Claim" }));

    await waitFor(() => expect(fake.calls).toContain("release"));
    const t = await fake.getTicket("acme", "acme-1");
    expect(t.state).toBe("Ready");
    expect(t.comments.at(-1)).toMatchObject({ actor: "human", body: "Session died overnight" });
    expect(t.claims[0]).toMatchObject({ ended_by: "release" });
  });

  it("releases without a note, and can be backed out of", async () => {
    const user = userEvent.setup();
    const fake = answeredWorld();
    renderApp(fake);
    const p = await panel();

    await user.click(within(p).getByRole("button", { name: "Release…" }));
    await user.click(within(p).getByRole("button", { name: "Keep the Claim" }));
    expect(within(p).queryByRole("alert")).toBeNull();

    await user.click(within(p).getByRole("button", { name: "Release…" }));
    await user.click(within(p).getByRole("button", { name: "Release Claim" }));
    await waitFor(() => expect(fake.calls).toContain("release"));
    const t = await fake.getTicket("acme", "acme-1");
    expect(t.state).toBe("Ready");
    expect(t.comments).toEqual([]);
  });

  it("shows the service's error if the release is refused", async () => {
    const user = userEvent.setup();
    const fake = silentWorld("sess-42");
    renderApp(fake);
    const p = await panel();
    // Someone else moved it on meanwhile; the notice hasn't arrived yet.
    fake.act = async () => {
      throw new ApiError(409, "action_not_allowed", "can't release acme-1: it is Done");
    };
    await user.click(within(p).getByRole("button", { name: "Stop…" }));
    await user.click(within(p).getByRole("button", { name: "Stop Claim" }));
    expect(await within(p).findByText(/can't release acme-1/)).toBeInTheDocument();
    expect(within(p).getByRole("button", { name: "Stop Claim" })).toBeEnabled();
  });
});
