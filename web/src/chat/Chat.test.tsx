// Chat in the app (Spec code-central-633), on the WASM Core: the icon by
// scope and Factory status, sending, a reply streaming in, and a pop-up
// that stays open when Chat stops being available.
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../App";
import { newWorld, type World } from "../api/core/core";
import { scenario } from "../api/scenario";

const beat = { factory: "code-central-factory", host: "laptop", interval_seconds: 30 } as const;

function renderApp(w: World, url = "/?project=acme") {
  window.history.replaceState(null, "", url);
  return render(<App api={w.api} now={w.now} />);
}

async function fresh(chat = true) {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  await s.reportHeartbeat("acme", { ...beat, state: "online", chat });
  return { w, s };
}

const icon = () => screen.findByRole("button", { name: "Chat" });
const popup = () => screen.findByRole("dialog", { name: "Chat" });

describe("the Chat icon", () => {
  it("shows when scoped to a Project with an Online Factory that has Chat support, and opens the pop-up", async () => {
    const { w } = await fresh();
    renderApp(w);

    await userEvent.click(await icon());

    const chat = await popup();
    expect(within(chat).getByRole("heading", { name: "Chat" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Chat" })).not.toBeInTheDocument();
    await userEvent.click(within(chat).getByRole("button", { name: "Close Chat" }));
    expect(await icon()).toBeInTheDocument();
  });

  it("is hidden in the all-Projects scope", async () => {
    const { w } = await fresh();
    renderApp(w, "/");

    await screen.findByRole("navigation", { name: "Filters" });
    await screen.findByRole("img", { name: "Factory Status: Online" });
    expect(screen.queryByRole("button", { name: "Chat" })).not.toBeInTheDocument();
  });

  it("is hidden when no Factory reports Chat support", async () => {
    const { w } = await fresh(false);
    renderApp(w);

    await screen.findByRole("img", { name: "Factory Status: Online" });
    expect(screen.queryByRole("button", { name: "Chat" })).not.toBeInTheDocument();
  });

  it.each(["draining", "stopping"] as const)("is hidden when the Factory with Chat support is %s", async (state) => {
    const { w, s } = await fresh();
    await s.reportHeartbeat("acme", { ...beat, state, chat: true });
    renderApp(w);

    await screen.findByRole("navigation", { name: "Filters" });
    await waitFor(() => expect(screen.queryByRole("img", { name: "Factory Status: Online" })).not.toBeInTheDocument());
    expect(screen.queryByRole("button", { name: "Chat" })).not.toBeInTheDocument();
  });
});

describe("a Conversation in the pop-up", () => {
  it("starts with the Chat row's defaults on the first message, then streams the reply", async () => {
    const { w, s } = await fresh();
    await w.api.setModels("acme", { default: { model: "", effort: "" }, chat: { model: "sonnet", effort: "low" }, kinds: [] });
    renderApp(w);
    await userEvent.click(await icon());
    const chat = await popup();

    await userEvent.type(within(chat).getByRole("textbox", { name: "Chat Message" }), "What is acme?");
    await userEvent.click(within(chat).getByRole("button", { name: "Send" }));

    expect(await within(chat).findByText("What is acme?")).toBeInTheDocument();
    expect(await within(chat).findByText("sonnet, low effort")).toBeInTheDocument();
    // A reply is running: Send is off and Stop is on.
    expect(await within(chat).findByRole("button", { name: "Stop" })).toBeInTheDocument();
    expect(within(chat).getByRole("button", { name: "Send" })).toBeDisabled();
    expect(within(chat).getByRole("button", { name: "Copy resume command" })).toBeDisabled();

    const conversation = await w.api.getChat("acme").then((c) => c.id!);
    const turn = await takeTurn(s, conversation);
    await s.chatReply("acme", turn, [
      { text: "Acme is a **demo**." },
      { kind: "tool_call", text: "Bash: ls", tool: "Bash", tool_use_id: "t1" },
      { kind: "tool_result", text: "README.md", tool: "Bash", tool_use_id: "t1" },
    ], { resume_command: "cd /w/chat && claude --resume session-1" });

    expect((await within(chat).findByText("demo")).tagName).toBe("STRONG");
    expect(within(chat).getByText("Bash: ls").closest("details")).not.toHaveAttribute("open");
    await waitFor(() => expect(within(chat).getByRole("button", { name: "Copy resume command" })).toBeEnabled());

    await s.chatReply("acme", turn, [{ text: " More." }], { outcome: "done" });
    await waitFor(() => expect(within(chat).queryByRole("button", { name: "Stop" })).not.toBeInTheDocument());
    await waitFor(() => expect(within(chat).getByRole("textbox", { name: "Chat Message" })).toBeEnabled());
  });

  it("stops a reply, and shows it Stopped", async () => {
    const { w } = await fresh();
    renderApp(w);
    await userEvent.click(await icon());
    const chat = await popup();
    await userEvent.type(within(chat).getByRole("textbox", { name: "Chat Message" }), "Hello");
    await userEvent.click(within(chat).getByRole("button", { name: "Send" }));

    // No Factory has taken the turn, so Stop ends the reply at once.
    await userEvent.click(await within(chat).findByRole("button", { name: "Stop" }));

    expect(await within(chat).findByText("Stopped")).toBeInTheDocument();
    expect(within(chat).queryByRole("button", { name: "Stop" })).not.toBeInTheDocument();
  });

  it("shows the Conversation as it stands when opened, the same as another tab would", async () => {
    const { w, s } = await fresh();
    await w.api.sendChatMessage("acme", "From another tab");
    const conversation = (await w.api.getChat("acme")).id!;
    await s.chatReply("acme", await takeTurn(s, conversation), [{ text: "An answer" }], { outcome: "done" });
    renderApp(w);

    await userEvent.click(await icon());

    const chat = await popup();
    expect(await within(chat).findByText("From another tab")).toBeInTheDocument();
    expect(await within(chat).findByText("An answer")).toBeInTheDocument();
  });

  it("starts a new conversation with the picked model, dropping the old one", async () => {
    const { w } = await fresh();
    await w.api.sendChatMessage("acme", "Old question");
    renderApp(w);
    await userEvent.click(await icon());
    const chat = await popup();
    expect(await within(chat).findByText("Old question")).toBeInTheDocument();

    await userEvent.click(within(chat).getByRole("button", { name: "New conversation" }));
    // With no Reported Models the picker is a plain textbox.
    const model = within(chat).getByRole("textbox", { name: "Model" });
    await userEvent.clear(model);
    await userEvent.type(model, "opus");
    await userEvent.click(within(chat).getByRole("button", { name: "Start" }));

    expect(await within(chat).findByText("opus")).toBeInTheDocument();
    await waitFor(() => expect(within(chat).queryByText("Old question")).not.toBeInTheDocument());
  });

  it("stays open and says why when its Factory stops being Online, and offers New conversation", async () => {
    const { w, s } = await fresh();
    await w.api.sendChatMessage("acme", "Hello");
    const conversation = (await w.api.getChat("acme")).id!;
    await s.chatReply("acme", await takeTurn(s, conversation), [{ text: "Hi" }], { outcome: "done" });
    renderApp(w);
    await userEvent.click(await icon());
    const chat = await popup();
    expect(await within(chat).findByText("Hi")).toBeInTheDocument();

    await s.reportHeartbeat("acme", { ...beat, state: "stopping", chat: true });

    expect(await within(chat).findByRole("status")).toHaveTextContent("This Conversation's Factory, code-central-factory on laptop, isn't Online");
    expect(within(chat).getByRole("button", { name: "Send" })).toBeDisabled();
    expect(within(chat).getByRole("button", { name: "New conversation" })).toBeEnabled();
    expect(within(chat).getByText("Hi")).toBeInTheDocument();
  });
});

// takeTurn has the Factory take the Conversation's pending turn and
// returns the Conversation it is for.
async function takeTurn(s: ReturnType<typeof scenario>, conversation: string): Promise<string> {
  const taken = await s.takeChatTurn("acme", "code-central-factory");
  expect(taken).toBe(conversation);
  return taken;
}
