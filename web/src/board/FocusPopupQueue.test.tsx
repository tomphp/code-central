// The Focus popup's Queue mode: the real App against FakeApi, with the
// popup rendered over it by a harness, as nothing opens it by itself yet.
// It shows Needs Me's Focus Items in scope one at a time, with Skip, N left
// and Open in Inbox, and reports why it closed and the Focus Item keys in
// scope then.
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import { App } from "../App";
import type { Api } from "../api/api";
import { FakeApi } from "../api/fake";
import type { RoundView } from "../api/types";
import { failCI } from "../ciTesting";
import { change } from "../focus/focusTesting";
import { useInboxData, useLiveVersion } from "../live";
import { hrefFor, readLocation, type UiLocation } from "../url";
import { FocusPopup, type FocusPopupClose } from "./FocusPopup";

// world has acme-1 waiting on Round 1 (Questions 1 and 2 open), acme-2
// Failed, beta-1 In Review and acme-3 In Review, in that Needs Me order.
function world() {
  const fake = new FakeApi();
  fake.addProject("acme", "Acme");
  fake.addProject("beta", "Beta");
  const rounds: RoundView[] = [
    {
      number: 1,
      claim: 1,
      posted: fake.now.toISOString(),
      questions: [
        { number: 1, title: "Which colour?", body: "Red or blue", recommended: "Blue", status: "open" },
        { number: 2, title: "Which size?", body: "S or M", status: "open" },
      ],
    },
  ];
  fake.addTicket("acme", { title: "Pick a colour", state: "Needs Answer", rounds }); // acme-1
  fake.addTicket("acme", { title: "Flaky build", state: "Failed" }); // acme-2
  fake.addTicket("acme", { title: "Login page", state: "In Review" }); // acme-3
  fake.addTicket("beta", { title: "Beta search", state: "In Review" }); // beta-1
  fake.setNeedsMe([
    { reason: "needs_answer", tickets: [{ id: "acme-1", round: { number: 1, open: 2, questions: 2 } }] },
    { reason: "failed", tickets: [{ id: "acme-2" }] },
    { reason: "in_review", tickets: [{ id: "beta-1", summary: "Added search" }, { id: "acme-3", summary: "Added the login page" }] },
  ]);
  return fake;
}

// tickets is world without its Round, so every Focus Item is a Ticket's.
function tickets() {
  const fake = world();
  fake.setNeedsMe(fake.needsMeGroups.filter((g) => g.reason !== "needs_answer"));
  return fake;
}

// Harness renders the popup in Queue mode while open, over the live Inbox
// data the App's views use, and navigates as the App does, by the
// address.
function Harness({ api, project, onClose }: { api: Api; project?: string; onClose(c: FocusPopupClose): void }) {
  const version = useLiveVersion(api);
  const inbox = useInboxData(api, version);
  const [open, setOpen] = useState(true);
  const navigate = (patch: Partial<UiLocation>) => {
    window.history.pushState(null, "", hrefFor({ ...readLocation(window.location.search), ...patch }));
    window.dispatchEvent(new PopStateEvent("popstate"));
  };
  if (!open || !inbox.data) return null;
  return (
    <FocusPopup
      mode="queue"
      api={api}
      now={(api as FakeApi).now}
      version={version}
      data={inbox.data}
      project={project}
      navigate={navigate}
      onClose={(c) => {
        onClose(c);
        setOpen(false);
      }}
    />
  );
}

// renderQueue renders the App at url with the Queue mode popup over it,
// scoped to project, and returns what it reports on closing.
function renderQueue(fake: FakeApi, { project, url = "/?view=activity" }: { project?: string; url?: string } = {}) {
  window.history.replaceState(null, "", url);
  const onClose = vi.fn<(c: FocusPopupClose) => void>();
  render(
    <>
      <App api={fake} now={() => fake.now} />
      <Harness api={fake} project={project} onClose={onClose} />
    </>,
  );
  return onClose;
}

const popup = () => screen.getByRole("dialog", { name: "Focus popup" });
const item = async () => within(await screen.findByRole("dialog", { name: "Focus popup" })).findByRole("article", { name: "Focus Item" });
const heading = async (name: string) => within(await screen.findByRole("dialog", { name: "Focus popup" })).findByRole("heading", { name });
const skip = async () => userEvent.click(await within(popup()).findByRole("button", { name: "Skip" }));
const noPopup = () => expect(screen.queryByRole("dialog", { name: "Focus popup" })).toBeNull();
const sorted = (keys: string[]) => [...keys].sort();

describe("the Focus popup's Queue mode", () => {
  it("shows Needs Me's first Focus Item on All Projects, with N left, Skip and Open in Inbox, and no + New ▾", async () => {
    renderQueue(tickets());
    expect(await heading("Flaky build")).toBeInTheDocument();
    expect(within(popup()).getByText("3 left")).toBeInTheDocument();
    expect(await within(popup()).findByRole("button", { name: "Skip" })).toBeInTheDocument();
    expect(within(popup()).getByRole("link", { name: "Open in Inbox" })).toHaveAttribute("href", "?view=inbox&ticket=acme-2");
    expect(within(popup()).getByRole("button", { name: "Close" })).toBeInTheDocument();
    expect(within(popup()).queryByRole("button", { name: /New/ })).toBeNull();
    expect(within(popup()).getAllByRole("article", { name: "Focus Item" })).toHaveLength(1);
  });

  it("goes through Needs Me's Focus Items in order with Skip, still counting each", async () => {
    renderQueue(tickets());
    await heading("Flaky build");
    await skip();
    expect(await heading("Beta search")).toBeInTheDocument();
    expect(within(popup()).getByText("3 left")).toBeInTheDocument();
    await skip();
    expect(await heading("Login page")).toBeInTheDocument();
  });

  it("is scoped to a Project, counting only its Focus Items", async () => {
    renderQueue(tickets(), { project: "beta" });
    expect(await heading("Beta search")).toBeInTheDocument();
    expect(within(popup()).getByText("1 left")).toBeInTheDocument();
  });

  it("closes by itself once only skipped Focus Items are left, reporting them as in scope", async () => {
    const fake = tickets();
    const closed = renderQueue(fake, { project: "acme" });
    await heading("Flaky build");
    await skip();
    await heading("Login page");
    const length = window.history.length;
    await skip();
    await waitFor(noPopup);
    expect(closed).toHaveBeenCalledOnce();
    const [{ why, keys }] = closed.mock.calls[0];
    expect(why).toBe("only_skipped");
    expect(sorted(keys)).toEqual(["acme-2 failed", "acme-3 in_review"]);
    expect(window.history.length).toBe(length);
    expect(fake.actions()).toEqual([]);
  });

  it("closes by itself once the queue is clear, with no keys left in scope", async () => {
    const fake = tickets();
    const closed = renderQueue(fake);
    await heading("Flaky build");
    change(fake, [], "acme-2");
    await waitFor(noPopup);
    expect(closed.mock.calls).toEqual([[{ why: "cleared", keys: [] }]]);
  });

  it("moves on quietly from a Focus Item dealt with elsewhere", async () => {
    const fake = tickets();
    renderQueue(fake);
    await heading("Flaky build");
    change(fake, fake.needsMeGroups.filter((g) => g.reason !== "failed"), "acme-2");
    expect(await heading("Beta search")).toBeInTheDocument();
    expect(within(popup()).getByText("2 left")).toBeInTheDocument();
    expect(screen.queryByText(/Dealt with elsewhere/)).toBeNull();
  });

  it("shows a Round's open Questions one at a time, as Focus Mode does, then the next Focus Item", async () => {
    const fake = world();
    renderQueue(fake, { project: "acme" });
    expect(await heading("Which colour?")).toBeInTheDocument();
    expect(within(popup()).getByText("Question 1 of 2 · Round 1")).toBeInTheDocument();
    expect(within(popup()).getByText("4 left")).toBeInTheDocument();
    expect(within(popup()).queryByText("Which size?")).toBeNull();
    await userEvent.click(within(popup()).getByRole("button", { name: "Answer" }));
    expect(await heading("Which size?")).toBeInTheDocument();
    expect(within(popup()).getByText("Question 2 of 2 · Round 1")).toBeInTheDocument();
    await skip();
    expect(await heading("Flaky build")).toBeInTheDocument();
    expect((await fake.getTicket("acme", "acme-1")).rounds[0].questions.map((q) => [q.status, q.answer])).toEqual([
      ["answered", "Blue"],
      ["open", undefined],
    ]);
  });

  it.each([
    ["×", () => userEvent.click(within(popup()).getByRole("button", { name: "Close" }))],
    ["Escape", () => userEvent.keyboard("{Escape}")],
    ["a click outside", () => userEvent.click(popup().parentElement!)],
  ])("closes on %s as a dismissal, reporting every Focus Item key in scope, and leaves the address alone", async (_, close) => {
    const closed = renderQueue(world(), { project: "acme" });
    await heading("Which colour?");
    const length = window.history.length;
    const search = window.location.search;
    await close();
    noPopup();
    expect(closed).toHaveBeenCalledOnce();
    const [{ why, keys }] = closed.mock.calls[0];
    expect(why).toBe("dismissed");
    // The read Round is keyed both as its row and as each open Question.
    expect(sorted(keys)).toEqual([
      "acme-1 round 1",
      "acme-1 round 1 question 1",
      "acme-1 round 1 question 2",
      "acme-2 failed",
      "acme-3 in_review",
    ]);
    expect(window.location.search).toBe(search);
    expect(window.history.length).toBe(length);
  });

  it("opens the Focus Item's Ticket in the Inbox from Open in Inbox, closing as a dismissal", async () => {
    const closed = renderQueue(tickets());
    await heading("Flaky build");
    await userEvent.click(within(await item()).getByRole("link", { name: "Open in Inbox" }));
    noPopup();
    expect(window.location.search).toBe("?view=inbox&ticket=acme-2");
    const detail = await screen.findByRole("complementary", { name: "Ticket" });
    expect(await within(detail).findByText("Flaky build")).toBeInTheDocument();
    expect(closed).toHaveBeenCalledOnce();
    expect(closed.mock.calls[0][0].why).toBe("dismissed");
    expect(sorted(closed.mock.calls[0][0].keys)).toEqual(["acme-2 failed", "acme-3 in_review", "beta-1 in_review"]);
  });

  it("includes CI items, Base diverged and Base missing", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    fake.addTicket("acme", { title: "Basket", state: "Accepted", branch: "acme-1", commit: "abc" }); // acme-1
    await failCI(fake, "acme", ["acme-1"]);
    fake.setNeedsMe([
      ...fake.needsMeGroups,
      { reason: "base_diverged", tickets: [{ id: "", project: "acme", title: "Base diverged", base_diverged: { base: "main", local: "aaa1111222", remote: "bbb2222333" } }] },
      { reason: "base_missing", tickets: [{ id: "", project: "acme", title: "Base missing", base_missing: { base: "release", fix: "Create the branch release" } }] },
    ]);
    renderQueue(fake);
    expect(await heading("CI failed at abc1234")).toBeInTheDocument();
    expect(within(popup()).getByText("3 left")).toBeInTheDocument();
    expect(await within(await item()).findByRole("region", { name: "CI failed" })).toBeInTheDocument();
    await skip();
    expect(await within(await item()).findByRole("region", { name: "Base diverged" })).toBeInTheDocument();
    await skip();
    expect(await within(await item()).findByRole("region", { name: "Base missing" })).toBeInTheDocument();
  });

  it("follows a new Focus Item arriving while it's open in N left, keeping the one on screen", async () => {
    const fake = tickets();
    renderQueue(fake);
    await heading("Flaky build");
    const t = fake.addTicket("acme", { title: "Quiet session", state: "In Progress" });
    await act(async () => change(fake, [{ reason: "silent_claim", tickets: [{ id: t.id }] }, ...fake.needsMeGroups], t.id));
    await waitFor(() => expect(within(popup()).getByText("4 left")).toBeInTheDocument());
    expect(within(popup()).getByRole("heading", { name: "Flaky build" })).toBeInTheDocument();
  });
});
