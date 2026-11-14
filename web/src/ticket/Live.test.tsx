// The Live section under each Claim in the Ticket detail, on a WASM Core
// world: open or closed by default, each Session Event kind and its marks,
// following the newest event, the tail note and the empty Transcript.
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { newWorld, type World } from "../api/core/core";
import { scenario, type Scenario } from "../api/scenario";
import type { SessionEvent, State } from "../api/types";
import { TicketDetail } from "./TicketDetail";

const at = (w: World, minutesAgo: number) => new Date(w.now().getTime() - minutesAgo * 60_000).toISOString();

// seeded is a Ticket whose first Claim, session s-old, was released and
// whose second, session s-new, is live; Needs Answer has it ask a Round.
async function seeded(state: State = "In Progress") {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  const id = await s.ticket("acme", { title: "Fix login" });
  await s.claim("acme", id, "s-old");
  await w.api.act("acme", id, "release", {});
  await s.claim("acme", id, "s-new");
  if (state === "Needs Answer") await s.askRound("acme", id);
  return { w, s, id };
}

async function renderDetail(w: World, id: string) {
  const t = await w.api.getTicket("acme", id);
  render(<TicketDetail api={w.api} project="acme" ticket={t} now={w.now()} />);
  const claims = within(screen.getByRole("article", { name: "Ticket details" })).getByRole("list", { name: "Claims" });
  return within(claims).getAllByRole("listitem");
}

// live is a Claim row's Live disclosure.
const live = (row: HTMLElement) => within(row).getByRole("group", { name: "Live" });
// transcript is an open Live section's Transcript.
const transcript = async (row: HTMLElement) => within(row).findByRole("log", { name: "Transcript" });

// post adds Session Events to a session as a Factory would, and lets the
// stream deliver them.
async function post(s: Scenario, session: string, events: SessionEvent[]) {
  await act(async () => s.postSessionEvents(session, events));
}

describe("the Live section", () => {
  it("is open by default for the live Claim of an In Progress Ticket, and streams its Transcript", async () => {
    const { w, s, id } = await seeded();
    await s.postSessionEvents("s-new", [{ time: at(w, 5), kind: "text", text: "I'll start with the **tests**." }]);
    const rows = await renderDetail(w, id);

    expect(live(rows[0])).toHaveAttribute("open");
    const log = await transcript(rows[0]);
    expect(await within(log).findByText("tests")).toContainHTML("<strong>tests</strong>");

    await post(s, "s-new", [{ time: at(w, 1), kind: "text", text: "Now the code." }]);
    expect(within(log).getByText("Now the code.")).toBeInTheDocument();
  });

  it("is closed by default for an ended Claim, and follows its Transcript only once opened", async () => {
    const { w, s, id } = await seeded();
    await s.postSessionEvents("s-old", [{ time: at(w, 245), kind: "factory", text: "acme-1: Claim ended" }]);
    const rows = await renderDetail(w, id);

    expect(live(rows[1])).not.toHaveAttribute("open");
    expect(within(rows[1]).queryByRole("log")).not.toBeInTheDocument();
    expect(w.transcriptStreams()).toBe(1);

    await userEvent.click(within(live(rows[1])).getByText("Live"));
    expect(await within(await transcript(rows[1])).findByText("acme-1: Claim ended")).toBeInTheDocument();
    expect(w.transcriptStreams()).toBe(2);

    await userEvent.click(within(live(rows[1])).getByText("Live"));
    expect(within(rows[1]).queryByRole("log")).not.toBeInTheDocument();
    expect(w.transcriptStreams()).toBe(1);
  });

  it("is closed by default for a live Claim when the Ticket isn't In Progress", async () => {
    const { w, id } = await seeded("Needs Answer");
    const rows = await renderDetail(w, id);
    expect(live(rows[0])).not.toHaveAttribute("open");
    expect(live(rows[1])).not.toHaveAttribute("open");
    expect(w.transcriptStreams()).toBe(0);
  });

  it("shows a tool call as one monospaced line, with its result collapsed beneath it and marked", async () => {
    const { w, s, id } = await seeded();
    await s.postSessionEvents("s-new", [
      { time: at(w, 9), kind: "tool_call", tool: "Bash", tool_use_id: "tu-1", text: "Bash: go test ./..." },
      { time: at(w, 8), kind: "tool_call", tool: "Read", tool_use_id: "tu-2", text: "Read: core/core.go" },
      { time: at(w, 7), kind: "tool_result", tool_use_id: "tu-1", text: "FAIL core 0.2s", is_error: true },
      { time: at(w, 6), kind: "tool_result", tool_use_id: "tu-2", text: "package core\n…", truncated: true },
    ]);
    const log = await transcript((await renderDetail(w, id))[0]);

    const bash = (await within(log).findByText("Bash: go test ./...")).closest(".transcript-row") as HTMLElement;
    expect(within(bash).getByText("Bash: go test ./...").tagName).toBe("CODE");
    const bashResult = within(bash).getByRole("group", { name: "Result" });
    expect(bashResult).not.toHaveAttribute("open");
    expect(within(bashResult).getByText("error")).toBeInTheDocument();
    expect(within(bashResult).queryByText("truncated")).not.toBeInTheDocument();
    expect(within(bashResult).getByText("FAIL core 0.2s")).not.toBeVisible();
    await userEvent.click(within(bashResult).getByText(/result/));
    expect(bashResult).toHaveAttribute("open");
    expect(within(bashResult).getByText("FAIL core 0.2s")).toBeVisible();

    const read = within(log).getByText("Read: core/core.go").closest(".transcript-row") as HTMLElement;
    const readResult = within(read).getByRole("group", { name: "Result" });
    expect(within(readResult).getByText("truncated")).toBeInTheDocument();
    expect(within(readResult).queryByText("error")).not.toBeInTheDocument();
    expect(within(log).getAllByRole("group", { name: "Result" })).toHaveLength(2);
  });

  it("shows a result on its own when its call isn't held", async () => {
    const { w, s, id } = await seeded();
    await s.postSessionEvents("s-new", [{ time: at(w, 7), kind: "tool_result", tool_use_id: "tu-0", text: "done" }]);
    const log = await transcript((await renderDetail(w, id))[0]);
    expect(await within(log).findByRole("group", { name: "Result" })).toHaveTextContent("done");
  });

  it("sets the Factory's lines apart from the agent's", async () => {
    const { w, s, id } = await seeded();
    await s.postSessionEvents("s-new", [
      { time: at(w, 9), kind: "factory", text: 'acme-1 claimed (implementation): "Fix login"' },
      { time: at(w, 8), kind: "text", text: "Starting." },
    ]);
    const log = await transcript((await renderDetail(w, id))[0]);
    const factory = (await within(log).findByText(/acme-1 claimed/)).closest(".transcript-row") as HTMLElement;
    expect(factory).toHaveClass("transcript-factory");
    expect(within(factory).getByText("Factory")).toBeInTheDocument();
    const agent = within(log).getByText("Starting.").closest(".transcript-row") as HTMLElement;
    expect(agent).not.toHaveClass("transcript-factory");
    expect(within(agent).queryByText("Factory")).not.toBeInTheDocument();
  });

  it("shows every row's time", async () => {
    const { w, s, id } = await seeded();
    const times = [at(w, 9), at(w, 8), at(w, 7), at(w, 6)];
    await s.postSessionEvents("s-new", [
      { time: times[0], kind: "factory", text: "claimed" },
      { time: times[1], kind: "text", text: "Starting." },
      { time: times[2], kind: "tool_call", tool: "Bash", tool_use_id: "tu-1", text: "Bash: ls" },
      { time: times[3], kind: "tool_result", tool_use_id: "tu-1", text: "go.mod" },
    ]);
    const log = await transcript((await renderDetail(w, id))[0]);
    await within(log).findByText("Bash: ls");
    const shown = [...log.querySelectorAll("time")];
    // Core keeps whole seconds.
    expect(shown.map((e) => e.getAttribute("datetime"))).toEqual(times.map((t) => t.replace(".000Z", "Z")));
    for (const e of shown) expect(e.textContent).toMatch(/^\d{2}:\d{2}:\d{2}/);
  });

  it("follows the newest event while at the bottom, pauses when scrolled up and resumes back at the bottom", async () => {
    const { w, s, id } = await seeded();
    const log = await transcript((await renderDetail(w, id))[0]);
    // jsdom lays nothing out: the log is 100px tall, its content 50px per event.
    Object.defineProperty(log, "clientHeight", { configurable: true, get: () => 100 });
    Object.defineProperty(log, "scrollHeight", {
      configurable: true,
      get: () => 50 * log.querySelectorAll(".transcript-row").length,
    });
    const say = (text: string) => post(s, "s-new", [{ time: w.now().toISOString(), kind: "text", text }]);

    await say("one");
    await say("two");
    await say("three");
    expect(log.scrollTop).toBe(150);

    log.scrollTop = 20;
    fireEvent.scroll(log);
    await say("four");
    expect(log.scrollTop).toBe(20);

    log.scrollTop = 100;
    fireEvent.scroll(log);
    await say("five");
    expect(log.scrollTop).toBe(250);
  });

  it("notes when the Transcript holds only the tail of the session", async () => {
    const { w, s, id } = await seeded();
    await s.postSessionEvents(
      "s-new",
      Array.from({ length: 2001 }, (_, i) => ({ time: at(w, 9), kind: "factory" as const, text: `line ${i}` })),
    );
    const log = await transcript((await renderDetail(w, id))[0]);
    expect(
      await within(log).findByText(
        "Earlier events aren't held (Code Central keeps the last 2,000 and none from before its last restart)",
      ),
    ).toBeInTheDocument();
    expect(within(log).queryByText("line 0")).not.toBeInTheDocument();
    expect(within(log).getByText("line 1")).toBeInTheDocument();
  });

  it("says when the Claim has no Transcript, and no tail note", async () => {
    const { w, s, id } = await seeded();
    const log = await transcript((await renderDetail(w, id))[0]);
    expect(await within(log).findByText("This Claim has no Transcript.")).toBeInTheDocument();
    expect(within(log).queryByText(/Earlier events/)).not.toBeInTheDocument();

    await post(s, "s-new", [{ time: at(w, 1), kind: "text", text: "Hello." }]);
    expect(within(log).queryByText("This Claim has no Transcript.")).not.toBeInTheDocument();
  });

  it("picks up after a dropped connection with no event lost or repeated", async () => {
    const { w, s, id } = await seeded();
    await s.postSessionEvents("s-new", [{ time: at(w, 3), kind: "text", text: "one" }]);
    const log = await transcript((await renderDetail(w, id))[0]);
    await within(log).findByText("one");

    await act(async () => w.dropStreams());
    await post(s, "s-new", [{ time: at(w, 2), kind: "text", text: "two" }]);
    await post(s, "s-new", [{ time: at(w, 1), kind: "text", text: "three" }]);
    // The feed reconnects after its retry delay.
    await waitFor(
      () =>
        expect([...log.querySelectorAll(".transcript-row")].map((r) => r.querySelector(".markdown")?.textContent)).toEqual([
          "one",
          "two",
          "three",
        ]),
      { timeout: 4000 },
    );
  });

  it("opens when its Ticket goes back to In Progress with the Claim still live", async () => {
    const { w, id } = await seeded("Needs Answer");
    const view = await w.api.getTicket("acme", id);
    const { rerender } = render(<TicketDetail api={w.api} project="acme" ticket={view} now={w.now()} />);
    const row = () => within(screen.getByRole("list", { name: "Claims" })).getAllByRole("listitem")[0];
    expect(live(row())).not.toHaveAttribute("open");

    rerender(<TicketDetail api={w.api} project="acme" ticket={{ ...view, state: "In Progress" }} now={w.now()} />);
    expect(live(row())).toHaveAttribute("open");
    expect(await transcript(row())).toBeInTheDocument();
  });
});
