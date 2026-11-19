// The Chat pop-up (Spec code-central-633): the Project's current
// Conversation with its header (model, effort, Copy resume command, New
// conversation), the Chat Messages, the notice for a Conversation or Chat
// that can't answer here, naming the host Chat runs on when that is
// another service, and the input with Send and Stop. It is presentational:
// the Conversation and the actions come in as props.
import { useEffect, useId, useState, type FormEvent } from "react";
import type { ApiError, Conversation, ModelRow, ReportedModels } from "../api/types";
import { Button } from "../controls/Button";
import { Select } from "../controls/Select";
import { CLAUDE_CODE_DEFAULT } from "../format";
import { CHAT_DEFAULT_MODEL, chatDefaults, effortOptions, modelSuggestions, ModelSuggestions } from "../modelPickers";
import { ChatMessages } from "./ChatMessages";

// NO_CHAT_FACTORY is the notice when nothing could answer a new Conversation.
export const NO_CHAT_FACTORY = "No Factory of this Project is Online with Chat support.";

export interface ChatPopupProps {
  // project is the Project's name, for the prompt to ask.
  project: string;
  // conversation is the Conversation as followed; undefined until the
  // stream first sends it.
  conversation?: Conversation;
  // chatAvailable is whether Chat is available for the Project here: an
  // Online Factory with Chat support reporting to this service. A
  // Conversation open when it turns false stays, with the reason.
  chatAvailable: boolean;
  // chatRow and reported fill New conversation's pickers.
  chatRow: ModelRow;
  reported: ReportedModels;
  // error is the last refusal of a Send, Stop or New conversation.
  error?: ApiError;
  // onSend rejects when the message is refused; the input then keeps its text.
  onSend(text: string): void | Promise<unknown>;
  onStop(): void;
  onStart(pick: ModelRow): void;
  onClose(): void;
  // copy writes the Resume Command to the clipboard; it defaults to the browser's.
  copy?(text: string): void | Promise<void>;
}

// ChatPopup is the pop-up itself; ChatDock holds the Conversation it shows.
export function ChatPopup(p: ChatPopupProps) {
  const [text, setText] = useState("");
  const [picking, setPicking] = useState(false);
  const [copied, setCopied] = useState(false);
  const c = p.conversation;
  const exists = !!c && c.availability !== "none";
  const unavailable = c?.availability === "unavailable";
  const running = !!c?.running;
  // Chat can't take a message when the Conversation can't be answered, or,
  // with none yet, when no Factory could start one.
  const noFactory = !exists && !p.chatAvailable;
  const cannotSend = !c || unavailable || noFactory;
  const reason = unavailable || noFactory ? cannotAnswerReason(c) : undefined;
  // A new Conversation has not been copied.
  useEffect(() => setCopied(false), [c?.id]);

  const send = (e: FormEvent) => {
    e.preventDefault();
    if (!text.trim() || cannotSend || running) return;
    // A refused message stays in the input to be sent again.
    Promise.resolve(p.onSend(text)).then(
      () => setText(""),
      () => {},
    );
  };
  const copyResume = async () => {
    if (!c?.resume_command) return;
    await (p.copy ?? ((t) => navigator.clipboard.writeText(t)))(c.resume_command);
    setCopied(true);
  };

  return (
    <section role="dialog" aria-label="Chat" className="chat-popup">
      <header className="chat-header">
        <h2>Chat</h2>
        {exists && (
          <span className="muted small chat-model" title={`${c.model}${c.effort ? `, ${c.effort} effort` : ""}`}>
            {c.model}
            {c.effort ? `, ${c.effort} effort` : ""}
          </span>
        )}
        <span className="chat-header-actions">
          {exists && (
            <Button
              type="button"
              disabled={!c.resume_command}
              title={c.resume_command ? undefined : "Available once the Conversation has a session, after its first reply"}
              onClick={copyResume}
            >
              {copied ? "Copied" : "Copy resume command"}
            </Button>
          )}
          <Button type="button" onClick={() => setPicking((v) => !v)} aria-expanded={picking}>
            New conversation
          </Button>
          <Button type="button" onClick={p.onClose} aria-label="Close Chat">
            ×
          </Button>
        </span>
      </header>
      {picking && (
        <NewConversation
          chatRow={p.chatRow}
          reported={p.reported}
          onStart={(pick) => {
            setPicking(false);
            setCopied(false);
            p.onStart(pick);
          }}
          onCancel={() => setPicking(false)}
        />
      )}
      {reason && (
        <p role="status" className="chat-notice">
          {reason}
        </p>
      )}
      {c ? <ChatMessages conversation={c} /> : <p className="muted small chat-note chat-loading">Loading the Conversation…</p>}
      {running && c?.messages.at(-1)?.from === "human" && (
        <p role="status" className="muted small chat-note">
          Working…
        </p>
      )}
      {exists && c.messages.length === 0 && !picking && <p className="muted small chat-note">Ask Claude about {p.project}.</p>}
      {!exists && c && !picking && <p className="muted small chat-note">Ask Claude about {p.project}: your first message starts a Conversation.</p>}
      {p.error && (
        <p role="alert" className="banner chat-refusal">
          {p.error.message}
        </p>
      )}
      <form className="chat-input" onSubmit={send}>
        <textarea
          aria-label="Chat Message"
          rows={2}
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) send(e);
          }}
        />
        {running && (
          <Button type="button" variant="danger" onClick={p.onStop}>
            Stop
          </Button>
        )}
        <Button type="submit" variant="primary" disabled={cannotSend || running || !text.trim()}>
          Send
        </Button>
      </form>
    </section>
  );
}

// cannotAnswerReason says why Chat can't answer here: that it runs on
// another service, or an unavailable Conversation's Factory, or that no
// Factory could.
function cannotAnswerReason(c: Conversation | undefined): string {
  if (c?.runs_on) return `Chat runs on ${c.runs_on}: no Factory with Chat support reports to this service.`;
  return c?.availability === "unavailable" && c.factory
    ? `This Conversation's Factory, ${c.factory.name} on ${c.factory.host}, isn't Online. Start a new conversation, or wait for it to come back.`
    : NO_CHAT_FACTORY;
}

// NewConversation asks for the model and effort the Conversation holds
// for its whole life, filled from the Chat row, then haiku and none.
function NewConversation({ chatRow, reported, onStart, onCancel }: { chatRow: ModelRow; reported: ReportedModels; onStart(pick: ModelRow): void; onCancel(): void }) {
  const defaults = chatDefaults({ chat: chatRow });
  const [model, setModel] = useState(defaults.model);
  const [effort, setEffort] = useState(defaults.effort);
  const id = useId();
  const suggestions = modelSuggestions(reported);
  return (
    <form
      className="chat-new"
      aria-label="New conversation"
      onSubmit={(e) => {
        e.preventDefault();
        onStart({ model: model.trim() || CHAT_DEFAULT_MODEL, effort });
      }}
    >
      <label>
        Model
        <input list={suggestions.length ? `${id}-models` : undefined} value={model} onChange={(e) => setModel(e.target.value)} />
      </label>
      <ModelSuggestions id={`${id}-models`} suggestions={suggestions} />
      <label>
        Effort
        <Select
          value={effort}
          onChange={setEffort}
          placeholder={`Default (${CLAUDE_CODE_DEFAULT})`}
          options={effortOptions(reported.effort_levels, effort).map((l) => ({ value: l }))}
        />
      </label>
      <Button type="submit" variant="primary">
        Start
      </Button>
      <Button type="button" onClick={onCancel}>
        Cancel
      </Button>
    </form>
  );
}
