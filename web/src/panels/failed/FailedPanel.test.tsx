// The Failed panel on the WASM Core: what it shows and which action it
// calls with what payload.
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../../App";
import { newWorld } from "../../api/core/core";
import { scenario } from "../../api/scenario";
import { FailedPanel } from "./panel";

// A Report is what an agent failed with, and the Claim it held: its
// session unless it had none, and where it ran.
interface Report {
  session?: string | null;
  working_dir?: string;
  resume_command?: string;
  what_broke?: string;
  why?: string;
  what_to_fix?: string;
  where_work_stands?: string;
}

const report = (n: number, over: Report = {}): Report => ({
  what_broke: `Broke ${n}\nthe symptom ${n}`,
  why: `Why ${n}`,
  what_to_fix: `Fix ${n}`,
  where_work_stands: `Stands ${n}`,
  ...over,
});

// world has acme's Fix login Failed once per report, the human sending it
// back between them.
async function world(reports: Report[] = [report(1), report(2)]) {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  const id = await s.ticket("acme", { title: "Fix login" });
  for (const [i, r] of reports.entries()) {
    const { session, working_dir, resume_command, ...fields } = r;
    const claim = await s.agent<{ claim_id: string }>(`${s.projectPath("acme")}/claim`, {
      ticket: id,
      ...(session !== null && { session_id: session ?? `sess-${i + 1}` }),
      ...(working_dir && { working_dir }),
      ...(resume_command && { resume_command }),
    });
    await s.agent(`${s.ticketPath("acme", id)}/report_failure`, { claim_id: claim.claim_id, ...fields });
    if (i < reports.length - 1) await s.human(`${s.ticketPath("acme", id)}/send_back`, {});
  }
  return { w, s, id };
}

type Failed = Awaited<ReturnType<typeof world>>;

async function renderPanel({ w, id }: Failed) {
  const ticket = await w.api.getTicket("acme", id);
  render(<FailedPanel api={w.api} project="acme" ticket={ticket} now={w.now()} />);
  return screen.getByRole("region", { name: "Failed" });
}

// renderInApp opens the Failed Ticket in the whole app's inbox and returns its Failed panel.
async function renderInApp({ w, id }: Failed) {
  window.history.replaceState(null, "", `/?view=inbox&ticket=${id}`);
  render(<App api={w.api} now={w.now} />);
  const pane = screen.getByRole("complementary", { name: "Ticket" });
  return within(pane).findByRole("region", { name: "Failed" });
}

describe("the Failed panel", () => {
  it("shows the failure count and the latest report's four fields", async () => {
    const panel = await renderPanel(await world());
    expect(panel).toHaveTextContent("Failed 2 times");
    const latest = within(panel).getByRole("group", { name: "Failure Report 2" });
    expect(within(latest).getByText("What broke").nextElementSibling).toHaveTextContent("Broke 2 the symptom 2");
    expect(within(latest).getByText("Why").nextElementSibling).toHaveTextContent("Why 2");
    expect(within(latest).getByText("What to fix").nextElementSibling).toHaveTextContent("Fix 2");
    expect(within(latest).getByText("Where the work stands").nextElementSibling).toHaveTextContent("Stands 2");
    expect(within(latest).queryByText(/Broke 1/)).toBeNull();
  });

  it("says once when it has failed once", async () => {
    const panel = await renderPanel(await world([report(1)]));
    expect(panel).toHaveTextContent("Failed once");
    expect(within(panel).queryByText(/Earlier reports/)).toBeNull();
  });

  it("keeps earlier reports collapsed below the moves", async () => {
    const panel = await renderPanel(await world([report(1), report(2), report(3)]));
    const earlier = within(panel).getByText("Earlier reports (2)").closest("details")!;
    expect(earlier).not.toHaveAttribute("open");
    expect(within(earlier).getByRole("group", { name: "Failure Report 2", hidden: true })).not.toBeVisible();
    expect(within(earlier).getByRole("group", { name: "Failure Report 1", hidden: true })).toBeInTheDocument();
    // After the moves: the Cancel button comes before the collapsed reports.
    const cancel = within(panel).getByRole("button", { name: "Cancel" });
    expect(cancel.compareDocumentPosition(earlier) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("shows the latest report's resume command for investigating", async () => {
    const panel = await renderPanel(await world());
    expect(within(panel).getByText("claude --resume sess-2")).toBeInTheDocument();
  });

  it("shows the latest report's copy of the Resume Command and working directory", async () => {
    const panel = await renderPanel(
      await world([report(1, { resume_command: "old" }), report(2, { working_dir: "/work/acme-1", resume_command: "agent resume sess-2" })]),
    );
    expect(within(panel).getByText("cd /work/acme-1 && agent resume sess-2")).toBeInTheDocument();
  });

  it("says when the latest report has no session to resume", async () => {
    const panel = await renderPanel(await world([report(1), report(2, { session: null })]));
    expect(within(panel).queryByText(/claude --resume/)).toBeNull();
    expect(panel).toHaveTextContent("No session ID is recorded");
  });

  it("sends back with the note", async () => {
    const x = await world();
    const panel = await renderPanel(x);
    await userEvent.type(within(panel).getByLabelText(/Note for the next agent/), "Fixed the fixture on main");
    await userEvent.click(within(panel).getByRole("button", { name: "Send back" }));

    expect(x.w.calls).toContain("send_back");
    const t = await x.w.api.getTicket("acme", x.id);
    expect(t.state).toBe("Ready");
    expect(t.comments.at(-1)).toMatchObject({ actor: "human", body: "Fixed the fixture on main" });
  });

  it("sends back with no note, saying that means nothing has changed", async () => {
    const x = await world();
    const panel = await renderPanel(x);
    expect(panel).toHaveTextContent("No note tells the next agent nothing has changed");
    const before = (await x.w.api.getTicket("acme", x.id)).comments;
    await userEvent.click(within(panel).getByRole("button", { name: "Send back" }));

    const t = await x.w.api.getTicket("acme", x.id);
    expect(t.state).toBe("Ready");
    expect(t.comments).toEqual(before);
  });

  it("moves to Draft", async () => {
    const x = await world();
    const panel = await renderPanel(x);
    await userEvent.click(within(panel).getByRole("button", { name: "To Draft" }));
    expect(x.w.calls).toContain("to_draft");
    expect((await x.w.api.getTicket("acme", x.id)).state).toBe("Draft");
  });

  it("cancels with no Replacement", async () => {
    const x = await world();
    const panel = await renderPanel(x);
    await userEvent.click(within(panel).getByRole("button", { name: "Cancel" }));
    expect(x.w.calls).toContain("cancel");
    const t = await x.w.api.getTicket("acme", x.id);
    expect(t).toMatchObject({ state: "Cancelled", replacements: [] });
  });

  it("shows a refusal and stays put", async () => {
    const x = await world();
    const panel = await renderPanel(x);
    await x.s.human(`${x.s.ticketPath("acme", x.id)}/send_back`, {}); // moved on elsewhere
    await userEvent.click(within(panel).getByRole("button", { name: "To Draft" }));
    expect(await within(panel).findByRole("alert")).toHaveTextContent("to_draft is not allowed");
  });

  it("renders every field of the latest and earlier reports as Markdown, under plain labels", async () => {
    const markdownFields = (n: number): Report => ({
      what_broke: `The **build ${n}** failed`,
      why: `- cause ${n}\n- second cause ${n}`,
      what_to_fix: `Run \`make ${n}\``,
      where_work_stands: `See [the branch ${n}](https://example.com/${n})`,
    });
    const panel = await renderInApp(await world([report(1, markdownFields(1)), report(2, markdownFields(2))]));
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
    const panel = await renderInApp(await world([report(1, { why: '<img src=x onerror="alert(1)"> <b>bold</b>' })]));
    const why = within(panel).getByText("Why").nextElementSibling!;
    expect(why.querySelector("img, b")).toBeNull();
    expect(why).toHaveTextContent('<img src=x onerror="alert(1)"> <b>bold</b>');
  });

  it("is the action panel for a Failed Ticket in the inbox", async () => {
    expect(await renderInApp(await world())).toHaveTextContent("Failed 2 times");
  });
});
