// The Focus popup's Queue mode: the real App on a WASM Core world, with the
// popup rendered over it by a harness, as nothing opens it by itself yet.
// It shows Needs Me's Focus Items in scope one at a time, with Skip, N left
// and Open in Inbox, and reports why it closed and the Focus Item keys in
// scope then, counted here: their format is the Focus session's own.
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import { App } from "../App";
import type { Api } from "../api/api";
import { newWorld, type World } from "../api/core/core";
import type { Scenario } from "../api/scenario";
import { failCI } from "../ciTesting";
import { Live, useInbox, useVersion } from "../live";
import { NeedsMeProvider } from "../needsme/context";
import { needsMeOf } from "../needsme/model";
import { hrefFor, readLocation, type UiLocation } from "../url";
import { scenario, settle } from "../worldTesting";
import { FocusPopup, type FocusPopupClose } from "./FocusPopup";

// Fake is a world, its scenario builder and the Tickets of its Projects.
interface Fake {
  w: World;
  s: Scenario;
  // colour waits on Round 1 (Questions 1 and 2 open) when the world has it.
  colour?: string;
  flaky: string;
  login: string;
  // search is beta's In Review Ticket, so a Project's scope leaves something out.
  search: string;
}

// tickets is acme with Flaky build Failed and Login page In Review, and
// beta with Beta search In Review, with All clear on: every Focus Item is a
// Ticket's.
async function tickets(): Promise<Fake> {
  const w = await newWorld();
  const s = scenario(w);
  await w.api.setOptions({ focus_clear: "all_clear" });
  await s.project("acme", "Acme");
  await s.project("beta", "Beta");
  const flaky = await s.ticket("acme", { title: "Flaky build", state: "Failed" });
  const login = await s.ticket("acme", { title: "Login page", state: "In Review" });
  const search = await s.ticket("beta", { title: "Beta search", state: "In Review" });
  return { w, s, flaky, login, search };
}

// world is tickets with acme's Pick a colour waiting on Round 1 as well.
async function world(): Promise<Fake> {
  const fake = await tickets();
  const colour = await fake.s.ticket("acme", { title: "Pick a colour" });
  await fake.s.askRound("acme", colour, [
    { title: "Which colour?", body: "Red or blue", recommended: "Blue" },
    { title: "Which size?", body: "S or M" },
  ]);
  return { ...fake, colour };
}

// withBase puts key under Merge + Push, under a Factory that is online,
// which has reported its base as state.
async function withBase({ s }: Fake, key: string, state: "diverged" | "missing") {
  await s.human(`${s.projectPath(key)}/set_landing_method`, { method: "merge", push: true, remote: "origin" });
  await s.reportHeartbeat(key, { factory: "code-central-factory", host: "laptop", state: "online", note: "", interval_seconds: 30 });
  await s.agent(`${s.projectPath(key)}/report_base`, state === "diverged" ? { state, local: "aaa1111222", remote: "bbb2222333" } : { state });
}

const sorted = (keys: string[]) => [...keys].sort();

// cancel has the human cancel the Tickets, so they need nothing.
async function cancel({ s }: Fake, key: string, ...ids: string[]) {
  for (const id of ids) await s.human(`${s.ticketPath(key, id)}/cancel`, {});
}

// Harness renders the popup in Queue mode while open, over the live Inbox
// data and its read model as the App's views have them, and navigates as
// the App does, by the address.
function Harness({ api, now, project, onClose }: { api: Api; now: Date; project?: string; onClose(c: FocusPopupClose): void }) {
  const version = useVersion();
  const inbox = useInbox();
  const [open, setOpen] = useState(true);
  const navigate = (patch: Partial<UiLocation>) => {
    window.history.pushState(null, "", hrefFor({ ...readLocation(window.location.search), ...patch }));
    window.dispatchEvent(new PopStateEvent("popstate"));
  };
  if (!open || !inbox.data) return null;
  return (
    <NeedsMeProvider value={needsMeOf(inbox.data)}>
      <FocusPopup
        mode="queue"
        now={now}
        data={inbox.data}
        project={project}
        navigate={navigate}
        onClose={(c) => {
          onClose(c);
          setOpen(false);
        }}
      />
    </NeedsMeProvider>
  );
}

// renderQueue renders the App at url with the Queue mode popup over it,
// scoped to project, settles, and returns what it reports on closing.
async function renderQueue({ w }: Fake, { project, url = "/?view=activity" }: { project?: string; url?: string } = {}) {
  window.history.replaceState(null, "", url);
  const onClose = vi.fn<(c: FocusPopupClose) => void>();
  render(
    <>
      <App api={w.api} now={w.now} />
      <Live api={w.api}>
        <Harness api={w.api} now={w.now()} project={project} onClose={onClose} />
      </Live>
    </>,
  );
  await settle(w);
  return onClose;
}

const popup = () => screen.getByRole("dialog", { name: "Focus popup" });
const item = () => within(popup()).getByRole("article", { name: "Focus Item" });
const heading = (name: string) => within(popup()).getByRole("heading", { name });
const skip = async ({ w }: Fake) => {
  await userEvent.click(within(popup()).getByRole("button", { name: "Skip" }));
  await settle(w);
};
const noPopup = () => expect(screen.queryByRole("dialog", { name: "Focus popup" })).toBeNull();

describe("the Focus popup's Queue mode", () => {
  it("shows Needs Me's first Focus Item on All Projects, with N left, Skip and Open in Inbox, and no + New ▾", async () => {
    const fake = await tickets();
    await renderQueue(fake);
    expect(heading("Flaky build")).toBeInTheDocument();
    expect(within(popup()).getByText("3 left")).toBeInTheDocument();
    expect(within(popup()).getByRole("button", { name: "Skip" })).toBeInTheDocument();
    expect(within(popup()).getByRole("link", { name: "Open in Inbox" })).toHaveAttribute("href", `?view=inbox&ticket=${fake.flaky}`);
    expect(within(popup()).getByRole("button", { name: "Close" })).toBeInTheDocument();
    expect(within(popup()).queryByRole("button", { name: /New/ })).toBeNull();
    expect(within(popup()).getAllByRole("article", { name: "Focus Item" })).toHaveLength(1);
  });

  it("goes through Needs Me's Focus Items in order with Skip, still counting each", async () => {
    const fake = await tickets();
    await renderQueue(fake);
    heading("Flaky build");
    await skip(fake);
    expect(heading("Login page")).toBeInTheDocument();
    expect(within(popup()).getByText("3 left")).toBeInTheDocument();
    await skip(fake);
    expect(heading("Beta search")).toBeInTheDocument();
  });

  it("is scoped to a Project, counting only its Focus Items", async () => {
    await renderQueue(await tickets(), { project: "beta" });
    expect(heading("Beta search")).toBeInTheDocument();
    expect(within(popup()).getByText("1 left")).toBeInTheDocument();
  });

  it("closes by itself once only skipped Focus Items are left, reporting them as in scope", async () => {
    const fake = await tickets();
    const closed = await renderQueue(fake, { project: "acme" });
    const actions = fake.w.actions();
    heading("Flaky build");
    await skip(fake);
    heading("Login page");
    const length = window.history.length;
    await skip(fake);
    noPopup();
    expect(closed).toHaveBeenCalledOnce();
    const [{ why, keys }] = closed.mock.calls[0];
    expect(why).toBe("only_skipped");
    expect(sorted(keys)).toEqual(sorted([`${fake.flaky} failed`, `${fake.login} in_review`]));
    expect(window.history.length).toBe(length);
    expect(fake.w.actions()).toEqual(actions);
  });

  it("closes by itself once the queue is clear, with no keys left in scope", async () => {
    const fake = await tickets();
    const closed = await renderQueue(fake);
    heading("Flaky build");
    await cancel(fake, "acme", fake.flaky, fake.login);
    await cancel(fake, "beta", fake.search);
    await settle(fake.w);
    noPopup();
    expect(closed.mock.calls).toEqual([[{ why: "cleared", keys: [] }]]);
  });

  it("moves on from a Focus Item dealt with elsewhere with Focus Mode's note", async () => {
    const fake = await tickets();
    await renderQueue(fake);
    heading("Flaky build");
    await cancel(fake, "acme", fake.flaky);
    await settle(fake.w);
    expect(heading("Login page")).toBeInTheDocument();
    expect(within(popup()).getByText("2 left")).toBeInTheDocument();
    expect(within(popup()).getByText("Dealt with elsewhere")).toBeInTheDocument();
  });

  it("shows a Round's open Questions one at a time, as Focus Mode does, then the next Focus Item", async () => {
    const fake = await world();
    await renderQueue(fake, { project: "acme" });
    expect(heading("Which colour?")).toBeInTheDocument();
    expect(within(popup()).getByText("Question 1 of 2 · Round 1")).toBeInTheDocument();
    expect(within(popup()).getByText("4 left")).toBeInTheDocument();
    expect(within(popup()).queryByText("Which size?")).toBeNull();
    await userEvent.click(within(popup()).getByRole("button", { name: "Answer" }));
    await settle(fake.w);
    expect(heading("Which size?")).toBeInTheDocument();
    expect(within(popup()).getByText("Question 2 of 2 · Round 1")).toBeInTheDocument();
    await skip(fake);
    expect(heading("Flaky build")).toBeInTheDocument();
    expect((await fake.w.api.getTicket("acme", fake.colour!)).rounds![0].questions.map((q) => [q.status, q.answer])).toEqual([
      ["answered", "Blue"],
      ["open", undefined],
    ]);
  });

  it.each([
    ["×", () => userEvent.click(within(popup()).getByRole("button", { name: "Close" }))],
    ["Escape", () => userEvent.keyboard("{Escape}")],
    ["a click outside", () => userEvent.click(popup().parentElement!)],
  ])("closes on %s as a dismissal, reporting every Focus Item key in scope, and leaves the address alone", async (_, close) => {
    const fake = await world();
    const closed = await renderQueue(fake, { project: "acme" });
    heading("Which colour?");
    const length = window.history.length;
    const search = window.location.search;
    await close();
    await settle(fake.w);
    noPopup();
    expect(closed).toHaveBeenCalledOnce();
    const [{ why, keys }] = closed.mock.calls[0];
    expect(why).toBe("dismissed");
    // The read Round is keyed both as its row and as each open Question.
    expect(sorted(keys)).toEqual(
      sorted([`${fake.colour} round 1`, `${fake.colour} round 1 question 1`, `${fake.colour} round 1 question 2`, `${fake.flaky} failed`, `${fake.login} in_review`]),
    );
    expect(window.location.search).toBe(search);
    expect(window.history.length).toBe(length);
  });

  it("opens the Focus Item's Ticket in the Inbox from Open in Inbox, closing as a dismissal", async () => {
    const fake = await tickets();
    const closed = await renderQueue(fake);
    heading("Flaky build");
    await userEvent.click(within(item()).getByRole("link", { name: "Open in Inbox" }));
    await settle(fake.w);
    noPopup();
    expect(window.location.search).toBe(`?view=inbox&ticket=${fake.flaky}`);
    const detail = screen.getByRole("complementary", { name: "Ticket" });
    expect(within(detail).getByText("Flaky build")).toBeInTheDocument();
    expect(closed).toHaveBeenCalledOnce();
    expect(closed.mock.calls[0][0].why).toBe("dismissed");
    expect(sorted(closed.mock.calls[0][0].keys)).toEqual(sorted([`${fake.flaky} failed`, `${fake.login} in_review`, `${fake.search} in_review`]));
  });

  // A Project has one reported base, so acme is Base diverged and beta
  // Base missing.
  it("includes CI items, Base diverged and Base missing", async () => {
    const fake = await world();
    const basket = await fake.s.ticket("acme", { title: "Basket", state: "Accepted" });
    await failCI(fake.s, "acme", [basket]);
    await withBase(fake, "acme", "diverged");
    await withBase(fake, "beta", "missing");
    await cancel(fake, "acme", fake.colour!, fake.flaky, fake.login);
    await cancel(fake, "beta", fake.search);
    await renderQueue(fake);
    expect(heading("CI failed at abc1234")).toBeInTheDocument();
    expect(within(popup()).getByText("3 left")).toBeInTheDocument();
    expect(within(item()).getByRole("region", { name: "CI failed" })).toBeInTheDocument();
    await skip(fake);
    expect(within(item()).getByRole("region", { name: "Base diverged" })).toBeInTheDocument();
    await skip(fake);
    expect(within(item()).getByRole("region", { name: "Base missing" })).toBeInTheDocument();
  });

  it("leaves out items of a reason off in their own Project, CI items, Base diverged and Base missing included", async () => {
    const fake = await world();
    const basket = await fake.s.ticket("acme", { title: "Basket", state: "Accepted" });
    await failCI(fake.s, "acme", [basket]);
    await withBase(fake, "acme", "diverged");
    await withBase(fake, "beta", "missing");
    await cancel(fake, "acme", fake.colour!, fake.flaky, fake.login);
    await cancel(fake, "beta", fake.search);
    await fake.w.api.setFocusKinds("acme", ["ci_failed", "base_diverged", "base_missing"]);
    const closed = await renderQueue(fake);
    expect(within(item()).getByRole("region", { name: "Base missing" })).toBeInTheDocument();
    expect(within(popup()).getByText("1 left")).toBeInTheDocument();
    await skip(fake);
    noPopup();
    expect(closed.mock.calls[0][0]).toEqual({ why: "only_skipped", keys: [expect.any(String)] });
  });

  it("closes by itself as cleared once only items of a reason off in their Project are left, with no keys in scope", async () => {
    const fake = await tickets();
    await fake.w.api.setFocusKinds("acme", ["in_review"]);
    const closed = await renderQueue(fake, { project: "acme" });
    heading("Flaky build");
    expect(within(popup()).getByText("1 left")).toBeInTheDocument();
    await cancel(fake, "acme", fake.flaky);
    await settle(fake.w);
    noPopup();
    expect(closed.mock.calls[0][0]).toEqual({ why: "cleared", keys: [] });
  });

  it("follows a new Focus Item arriving while it's open in N left, keeping the one on screen", async () => {
    const fake = await tickets();
    await renderQueue(fake);
    heading("Flaky build");
    await fake.s.ticket("acme", { title: "Quiet session", state: "In Review" });
    await settle(fake.w);
    expect(within(popup()).getByText("4 left")).toBeInTheDocument();
    expect(within(popup()).getByRole("heading", { name: "Flaky build" })).toBeInTheDocument();
  });
});
