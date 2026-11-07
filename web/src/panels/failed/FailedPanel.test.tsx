// The Failed panel, against FakeApi: what it shows and which action it
// calls with what payload.
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../../App";
import { FakeApi } from "../../api/fake";
import type { FailureReportView } from "../../api/types";
import { FailedPanel } from "./panel";

const report = (n: number, over: Partial<FailureReportView> = {}): FailureReportView => ({
  number: n,
  claim: n,
  session_id: `sess-${n}`,
  actor: "agent",
  time: "2026-11-07T08:00:00Z",
  what_broke: `Broke ${n}\nthe symptom ${n}`,
  why: `Why ${n}`,
  what_to_fix: `Fix ${n}`,
  where_work_stands: `Stands ${n}`,
  ...over,
});

function world(reports: FailureReportView[] = [report(1), report(2)]) {
  const fake = new FakeApi();
  fake.addProject("acme", "Acme");
  fake.addTicket("acme", { title: "Fix login", state: "Failed", failure_reports: reports }); // acme-1
  fake.setNeedsMe([{ reason: "failed", tickets: [{ id: "acme-1", failure: { headline: "Broke", reports: reports.length } }] }]);
  return fake;
}

async function renderPanel(fake: FakeApi) {
  const ticket = await fake.getTicket("acme", "acme-1");
  render(<FailedPanel api={fake} project="acme" ticket={ticket} now={fake.now} />);
  return screen.getByRole("region", { name: "Failed" });
}

describe("the Failed panel", () => {
  it("shows the failure count and the latest report's four fields", async () => {
    const panel = await renderPanel(world());
    expect(panel).toHaveTextContent("Failed 2 times");
    const latest = within(panel).getByRole("group", { name: "Failure Report 2" });
    expect(within(latest).getByText("What broke").nextElementSibling).toHaveTextContent("Broke 2 the symptom 2");
    expect(within(latest).getByText("Why").nextElementSibling).toHaveTextContent("Why 2");
    expect(within(latest).getByText("What to fix").nextElementSibling).toHaveTextContent("Fix 2");
    expect(within(latest).getByText("Where the work stands").nextElementSibling).toHaveTextContent("Stands 2");
    expect(within(latest).queryByText(/Broke 1/)).toBeNull();
  });

  it("says once when it has failed once", async () => {
    const panel = await renderPanel(world([report(1)]));
    expect(panel).toHaveTextContent("Failed once");
    expect(within(panel).queryByText(/Earlier reports/)).toBeNull();
  });

  it("keeps earlier reports collapsed below the moves", async () => {
    const panel = await renderPanel(world([report(1), report(2), report(3)]));
    const earlier = within(panel).getByText("Earlier reports (2)").closest("details")!;
    expect(earlier).not.toHaveAttribute("open");
    expect(within(earlier).getByRole("group", { name: "Failure Report 2", hidden: true })).not.toBeVisible();
    expect(within(earlier).getByRole("group", { name: "Failure Report 1", hidden: true })).toBeInTheDocument();
    // After the moves: the Cancel button comes before the collapsed reports.
    const cancel = within(panel).getByRole("button", { name: "Cancel" });
    expect(cancel.compareDocumentPosition(earlier) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("shows the latest report's resume command for investigating", async () => {
    const panel = await renderPanel(world());
    expect(within(panel).getByText("claude --resume sess-2")).toBeInTheDocument();
  });

  it("says when the latest report has no session to resume", async () => {
    const panel = await renderPanel(world([report(1), report(2, { session_id: undefined })]));
    expect(within(panel).queryByText(/claude --resume/)).toBeNull();
    expect(panel).toHaveTextContent("No session ID is recorded");
  });

  it("sends back with the note", async () => {
    const fake = world();
    const panel = await renderPanel(fake);
    await userEvent.type(within(panel).getByLabelText(/Note for the next agent/), "Fixed the fixture on main");
    await userEvent.click(within(panel).getByRole("button", { name: "Send back" }));

    expect(fake.calls).toContain("send_back");
    const t = await fake.getTicket("acme", "acme-1");
    expect(t.state).toBe("Ready");
    expect(t.comments.at(-1)).toMatchObject({ actor: "human", body: "Fixed the fixture on main" });
  });

  it("sends back with no note, saying that means nothing has changed", async () => {
    const fake = world();
    const panel = await renderPanel(fake);
    expect(panel).toHaveTextContent("No note tells the next agent nothing has changed");
    await userEvent.click(within(panel).getByRole("button", { name: "Send back" }));

    const t = await fake.getTicket("acme", "acme-1");
    expect(t.state).toBe("Ready");
    expect(t.comments).toEqual([]);
  });

  it("moves to Draft", async () => {
    const fake = world();
    const panel = await renderPanel(fake);
    await userEvent.click(within(panel).getByRole("button", { name: "To Draft" }));
    expect(fake.calls).toContain("to_draft");
    expect((await fake.getTicket("acme", "acme-1")).state).toBe("Draft");
  });

  it("cancels with no Replacement", async () => {
    const fake = world();
    const panel = await renderPanel(fake);
    await userEvent.click(within(panel).getByRole("button", { name: "Cancel" }));
    expect(fake.calls).toContain("cancel");
    const t = await fake.getTicket("acme", "acme-1");
    expect(t).toMatchObject({ state: "Cancelled", replacements: [] });
  });

  it("shows a refusal and stays put", async () => {
    const fake = world();
    const panel = await renderPanel(fake);
    fake.updateTicket("acme-1", { state: "Ready" }); // moved on elsewhere
    await userEvent.click(within(panel).getByRole("button", { name: "To Draft" }));
    expect(await within(panel).findByRole("alert")).toHaveTextContent("to_draft is not allowed");
  });

  it("is the action panel for a Failed Ticket in the inbox", async () => {
    window.history.replaceState(null, "", "/?view=inbox&ticket=acme-1");
    const fake = world();
    render(<App api={fake} now={() => fake.now} />);
    const pane = screen.getByRole("complementary", { name: "Ticket" });
    expect(await within(pane).findByRole("region", { name: "Failed" })).toHaveTextContent("Failed 2 times");
  });
});
