// The Chat pop-up (Spec code-central-633), through its rendered output:
// Chat Messages as plain text and Markdown, collapsed tool lines, the
// warning, ending and tail notices, Send and Stop, the unavailable notice,
// New conversation's pickers and Copy resume command.
import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ApiError, type Conversation } from "../api/types";
import { options, suggested } from "../testPickers";
import { claude, conversation, human, item, noConversation } from "./chatFixtures";
import { ChatPopup, NO_CHAT_FACTORY, type ChatPopupProps } from "./ChatPopup";

function renderPopup(c: Conversation | undefined, props: Partial<ChatPopupProps> = {}) {
  const handlers = { onSend: vi.fn(), onStop: vi.fn(), onStart: vi.fn(), onClose: vi.fn() };
  render(
    <ChatPopup
      project="Acme"
      conversation={c}
      chatAvailable
      chatRow={{ model: "", effort: "" }}
      reported={{ effort_levels: ["low", "high"], aliases: ["sonnet"], model_ids: [], model_flag: "", effort_flag: "" }}
      {...handlers}
      {...props}
    />,
  );
  return handlers;
}

const log = () => screen.getByRole("log", { name: "Conversation" });
const send = () => screen.getByRole("button", { name: "Send" });
const typeMessage = (text: string) => userEvent.type(screen.getByRole("textbox", { name: "Chat Message" }), text);

describe("the Chat pop-up's messages", () => {
  it("shows the human's messages as plain text and Claude's as Markdown", () => {
    renderPopup(conversation([human("What is **acme**?"), claude([item("text", "It is **a project** with `code`.")], { outcome: "done" })]));

    expect(within(log()).getByText("What is **acme**?")).toBeInTheDocument();
    expect(within(log()).getByText("a project").tagName).toBe("STRONG");
    expect(within(log()).getByText("code").tagName).toBe("CODE");
  });

  it("shows each tool call as one collapsed line that opens to its result", async () => {
    renderPopup(
      conversation([
        human("Look"),
        claude([item("tool_call", "Bash: ls", { tool: "Bash", tool_use_id: "t1" }), item("tool_result", "README.md", { tool: "Bash", tool_use_id: "t1" })], { outcome: "done" }),
      ]),
    );

    const line = within(log()).getByText("Bash: ls").closest("details")!;
    expect(line).not.toHaveAttribute("open");
    // The result is under its call, not a line of its own.
    expect(within(log()).getAllByRole("group")).toHaveLength(1);
    await userEvent.click(within(line).getByText("Bash: ls"));
    expect(line).toHaveAttribute("open");
    expect(within(line).getByText("README.md")).toBeInTheDocument();
  });

  it("marks a failed or cut tool result", () => {
    renderPopup(
      conversation([
        claude([item("tool_call", "Bash: x", { tool_use_id: "t1" }), item("tool_result", "boom", { tool_use_id: "t1", is_error: true, truncated: true })], { outcome: "done" }),
      ]),
    );

    expect(within(log()).getByText("error")).toBeInTheDocument();
    expect(within(log()).getByText("truncated")).toBeInTheDocument();
  });

  it("shows a result whose call was not held on its own line", () => {
    renderPopup(conversation([claude([item("tool_result", "orphan", { tool: "Read", tool_use_id: "gone" })], { outcome: "done" })]));

    expect(within(log()).getByText("Read result")).toBeInTheDocument();
  });

  it("shows permission denials as a warning", () => {
    renderPopup(conversation([claude([item("permission_denials", "Bash(rm -rf /)")], { outcome: "done" })]));

    expect(within(log()).getByRole("alert")).toHaveTextContent("Permission denied. Bash(rm -rf /)");
  });

  it("ends a stopped reply with Stopped and a failed one with its error", () => {
    renderPopup(
      conversation([
        human("one"),
        claude([item("text", "partial")], { outcome: "stopped" }),
        human("two"),
        claude([], { outcome: "failed", error: "model 'gpt' was rejected" }),
      ]),
    );

    expect(within(log()).getByText("Stopped")).toBeInTheDocument();
    expect(within(log()).getByRole("alert")).toHaveTextContent("Error: model 'gpt' was rejected");
  });

  it("says Claude is working on a reply that has no items yet", () => {
    renderPopup(conversation([human("one"), claude([])], { running: true }));

    expect(within(log()).getByText("Claude is working on it…")).toBeInTheDocument();
  });

  it("shows Working… while only the human's message is there", () => {
    renderPopup(conversation([human("one")], { running: true }));

    expect(screen.getByText("Working…")).toBeInTheDocument();
  });

  it("says older messages were dropped, only for a tail", () => {
    renderPopup(conversation([human("late")], { tail: true }));

    expect(within(log()).getByText("Older messages were dropped")).toBeInTheDocument();
  });

  it("shows no dropped notice for a whole Conversation", () => {
    renderPopup(conversation([human("early")]));

    expect(screen.queryByText("Older messages were dropped")).not.toBeInTheDocument();
  });
});

describe("Send and Stop", () => {
  it("sends the typed message and clears the input", async () => {
    const h = renderPopup(conversation([]));

    expect(send()).toBeDisabled();
    await typeMessage("Hello there");
    await userEvent.click(send());

    expect(h.onSend).toHaveBeenCalledWith("Hello there");
    await act(async () => {});
    expect(screen.getByRole("textbox", { name: "Chat Message" })).toHaveValue("");
  });

  it("keeps the typed message in the input when it is refused", async () => {
    const onSend = vi.fn().mockRejectedValue(new ApiError(409, "action_not_allowed", "a reply is running"));
    renderPopup(conversation([]), { onSend });

    await typeMessage("Hello there");
    await userEvent.click(send());

    expect(onSend).toHaveBeenCalledOnce();
    expect(screen.getByRole("textbox", { name: "Chat Message" })).toHaveValue("Hello there");
  });

  it("sends on Enter, and keeps a newline for Shift+Enter", async () => {
    const h = renderPopup(conversation([]));

    await typeMessage("one{Shift>}{Enter}{/Shift}two{Enter}");

    expect(h.onSend).toHaveBeenCalledOnce();
    expect(h.onSend).toHaveBeenCalledWith("one\ntwo");
  });

  it("disables Send and shows Stop while a reply runs", async () => {
    const h = renderPopup(conversation([human("hi"), claude([])], { running: true }));

    await typeMessage("more");
    expect(send()).toBeDisabled();
    await userEvent.click(screen.getByRole("button", { name: "Stop" }));
    expect(h.onStop).toHaveBeenCalledOnce();
  });

  it("shows no Stop when no reply runs", () => {
    renderPopup(conversation([]));

    expect(screen.queryByRole("button", { name: "Stop" })).not.toBeInTheDocument();
  });

  it("lets the first message start a Conversation when there is none and Chat is available", async () => {
    const h = renderPopup(noConversation());

    await typeMessage("Hello");
    await userEvent.click(send());

    expect(h.onSend).toHaveBeenCalledWith("Hello");
  });

  it("shows a refusal", () => {
    renderPopup(conversation([]), { error: new ApiError(409, "action_not_allowed", "a reply is running") });

    expect(screen.getByRole("alert")).toHaveTextContent("a reply is running");
  });
});

describe("when the Conversation can't answer", () => {
  it("says its Factory isn't Online, disables Send and still offers New conversation", async () => {
    renderPopup(conversation([human("hi")], { availability: "unavailable", factory: { name: "code-central-factory", host: "laptop" } }), { chatAvailable: false });

    expect(screen.getByRole("status")).toHaveTextContent("This Conversation's Factory, code-central-factory on laptop, isn't Online");
    await typeMessage("hello");
    expect(send()).toBeDisabled();
    expect(screen.getByRole("button", { name: "New conversation" })).toBeEnabled();
    // What was said stays readable.
    expect(within(log()).getByText("hi")).toBeInTheDocument();
  });

  it("says no Factory has Chat support when there is no Conversation and Chat stopped being available", async () => {
    renderPopup(noConversation(), { chatAvailable: false });

    expect(screen.getByRole("status")).toHaveTextContent(NO_CHAT_FACTORY);
    await typeMessage("hello");
    expect(send()).toBeDisabled();
  });

  it("says the host Chat runs on when no Factory with Chat support reports to this service, and disables Send", async () => {
    renderPopup(noConversation({ runs_on: "desktop" }), { chatAvailable: false });

    expect(screen.getByRole("status")).toHaveTextContent("Chat runs on desktop: no Factory with Chat support reports to this service.");
    await typeMessage("hello");
    expect(send()).toBeDisabled();
  });

  it("says the host Chat runs on for a Conversation this service can't answer", () => {
    renderPopup(conversation([human("hi")], { availability: "unavailable", factory: { name: "code-central-factory", host: "laptop" }, runs_on: "desktop" }), {
      chatAvailable: false,
    });

    expect(screen.getByRole("status")).toHaveTextContent("Chat runs on desktop");
    expect(send()).toBeDisabled();
  });

  it("says it is loading until the Conversation first arrives", () => {
    renderPopup(undefined);

    expect(screen.getByText("Loading the Conversation…")).toBeInTheDocument();
    expect(send()).toBeDisabled();
  });
});

describe("the header", () => {
  it("shows the Conversation's model and effort", () => {
    renderPopup(conversation([], { model: "opus", effort: "high" }));

    expect(screen.getByText("opus, high effort")).toBeInTheDocument();
  });

  it("disables Copy resume command until there is a session", () => {
    renderPopup(conversation([human("hi")]));

    expect(screen.getByRole("button", { name: "Copy resume command" })).toBeDisabled();
  });

  it("copies the Resume Command", async () => {
    const copy = vi.fn();
    renderPopup(conversation([human("hi")], { resume_command: "cd /w/chat && claude --resume s1" }), { copy });

    await userEvent.click(screen.getByRole("button", { name: "Copy resume command" }));

    expect(copy).toHaveBeenCalledWith("cd /w/chat && claude --resume s1");
    await act(async () => {});
    expect(screen.getByRole("button", { name: "Copied" })).toBeInTheDocument();
  });

  it("closes", async () => {
    const h = renderPopup(conversation([]));

    await userEvent.click(screen.getByRole("button", { name: "Close Chat" }));

    expect(h.onClose).toHaveBeenCalledOnce();
  });
});

describe("New conversation", () => {
  const form = () => screen.getByRole("form", { name: "New conversation" });
  const open = () => userEvent.click(screen.getByRole("button", { name: "New conversation" }));

  it("offers the Reported Models and the valid efforts, defaulting to haiku and no effort when the Chat row is empty", async () => {
    renderPopup(conversation([human("hi")]));
    await open();

    const model = within(form()).getByRole("combobox", { name: "Model" });
    expect(model).toHaveValue("haiku");
    expect(suggested(model)).toEqual(["sonnet"]);
    const effort = within(form()).getByRole("combobox", { name: "Effort" });
    expect(effort).toHaveValue("");
    expect(options(effort)).toEqual(["Default (Claude Code's default)", "low", "high"]);
  });

  it("defaults to the Chat row's model and effort", async () => {
    renderPopup(conversation([]), { chatRow: { model: "opus", effort: "high" } });
    await open();

    expect(within(form()).getByRole("combobox", { name: "Model" })).toHaveValue("opus");
    expect(within(form()).getByRole("combobox", { name: "Effort" })).toHaveValue("high");
  });

  it("starts with the picked model and effort", async () => {
    const h = renderPopup(conversation([human("hi")]));
    await open();

    const model = within(form()).getByRole("combobox", { name: "Model" });
    await userEvent.clear(model);
    await userEvent.type(model, "sonnet");
    await userEvent.selectOptions(within(form()).getByRole("combobox", { name: "Effort" }), "low");
    await userEvent.click(within(form()).getByRole("button", { name: "Start" }));

    expect(h.onStart).toHaveBeenCalledWith({ model: "sonnet", effort: "low" });
    expect(screen.queryByRole("form", { name: "New conversation" })).not.toBeInTheDocument();
  });

  it("falls back to haiku when the model is cleared, and can be cancelled", async () => {
    const h = renderPopup(conversation([]));
    await open();
    await userEvent.clear(within(form()).getByRole("combobox", { name: "Model" }));
    await userEvent.click(within(form()).getByRole("button", { name: "Start" }));
    expect(h.onStart).toHaveBeenCalledWith({ model: "haiku", effort: "" });

    await open();
    await userEvent.click(within(form()).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("form", { name: "New conversation" })).not.toBeInTheDocument();
  });
});
