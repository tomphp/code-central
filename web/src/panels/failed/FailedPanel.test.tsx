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

// renderInApp opens acme-1 in the whole app's inbox and returns its Failed panel.
async function renderInApp(fake: FakeApi) {
  window.history.replaceState(null, "", "/?view=inbox&ticket=acme-1");
  render(<App api={fake} now={() => fake.now} />);
  const pane = screen.getByRole("complementary", { name: "Ticket" });
  return within(pane).findByRole("region", { name: "Failed" });
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

  it("shows the latest report's copy of the Resume Command and working directory", async () => {
    const panel = await renderPanel(
      world([report(1, { resume_command: "old" }), report(2, { working_dir: "/work/acme-1", resume_command: "agent resume sess-2" })]),
    );
    expect(within(panel).getByText("cd /work/acme-1 && agent resume sess-2")).toBeInTheDocument();
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

  it("renders every field of the latest and earlier reports as Markdown, under plain labels", async () => {
    const markdownFields = (n: number): Partial<FailureReportView> => ({
      what_broke: `The **build ${n}** failed`,
      why: `- cause ${n}\n- second cause ${n}`,
      what_to_fix: `Run \`make ${n}\``,
      where_work_stands: `See [the branch ${n}](https://example.com/${n})`,
    });
    const panel = await renderInApp(world([report(1, markdownFields(1)), report(2, markdownFields(2))]));
    const earlier = within(panel).getByText("Earlier reports (1)").closest("details")!;
    const groups = [
      within(panel).getByRole("group", { name: "Failure Report 2" }),
      within(earlier).getByRole("group", { name: "Failure Report 1", hidden: true }),
    ];

    groups.forEach((group, i) => {
      const n = 2 - i;
      const field = (label: string) => {
        const dt = within(group).getByText(label);
        expect(dt.tagName).toBe("DT");
        expect(dt.children).toHaveLength(0); // the label stays plain text
        const dd = dt.nextElementSibling as HTMLElement;
        expect(dd.tagName).toBe("DD");
        return dd;
      };
      expect(field("What broke").querySelector("strong")).toHaveTextContent(`build ${n}`);
      expect(field("What broke")).not.toHaveTextContent("**");
      expect(within(field("Why")).getAllByRole("listitem", { hidden: true })).toHaveLength(2);
      expect(field("What to fix").querySelector("code")).toHaveTextContent(`make ${n}`);
      const link = within(field("Where the work stands")).getByRole("link", { name: `the branch ${n}`, hidden: true });
      expect(link).toHaveAttribute("href", `https://example.com/${n}`);
      expect(link).toHaveAttribute("target", "_blank");
    });
  });

  it("shows raw HTML in a report field as text", async () => {
    const panel = await renderInApp(world([report(1, { why: '<img src=x onerror="alert(1)"> <b>bold</b>' })]));
    const why = within(panel).getByText("Why").nextElementSibling!;
    expect(why.querySelector("img, b")).toBeNull();
    expect(why).toHaveTextContent('<img src=x onerror="alert(1)"> <b>bold</b>');
  });

  it("is the action panel for a Failed Ticket in the inbox", async () => {
    expect(await renderInApp(world())).toHaveTextContent("Failed 2 times");
  });
});
