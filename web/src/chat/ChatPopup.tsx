// The Chat pop-up (Spec code-central-633): the Project's current
// Conversation with its header (model, effort, Copy resume command, New
// conversation), the Chat Messages, the notice for a Conversation or Chat
// that can't answer, and the input with Send and Stop. It is presentational:
// the Conversation and the actions come in as props.
import { useId, useState, type FormEvent } from "react";
import type { ApiError, Conversation, ModelRow, ReportedModels } from "../api/types";
import { Button } from "../controls/Button";
import { Select } from "../controls/Select";
import { CLAUDE_CODE_DEFAULT } from "../format";
import { CHAT_DEFAULT_MODEL, chatDefaults, effortOptions, modelSuggestions, ModelSuggestions } from "../modelPickers";
import { ChatMessages } from "./ChatMessages";

export const NO_CHAT_FACTORY = "No Factory of this Project is Online with Chat support.";

export interface ChatPopupProps {
  project: string;
  // conversation is the Conversation as followed; undefined until the
  // stream first sends it.
  conversation?: Conversation;
  // chatAvailable is whether Chat is available for the Project: an Online
  // Factory with Chat support. A Conversation open when it turns false
  // stays, with the reason.
  chatAvailable: boolean;
  // chatRow and reported fill New conversation's pickers.
  chatRow: ModelRow;
  reported: ReportedModels;
  // error is the last refusal of a Send, Stop or New conversation.
  error?: ApiError;
  onSend(text: string): void;
  onStop(): void;
  onStart(pick: ModelRow): void;
  onClose(): void;
  // copy writes the Resume Command to the clipboard; it defaults to the browser's.
  copy?(text: string): void | Promise<void>;
}

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
  const cannotSend = !c || unavailable || (!exists && !p.chatAvailable);
  const reason = unavailable ? unavailableReason(c) : !exists && !p.chatAvailable ? NO_CHAT_FACTORY : undefined;

  const send = (e: FormEvent) => {
    e.preventDefault();
    if (!text.trim() || cannotSend || running) return;
    p.onSend(text);
    setText("");
  };
  const copy = async () => {
    if (!c?.resume_command) return;
    await (p.copy ?? ((t) => navigator.clipboard.writeText(t)))(c.resume_command);
    setCopied(true);
  };

  return (
    <section role="dialog" aria-label="Chat" className="chat-popup">
      <header className="chat-header">
        <h2>Chat</h2>
        {exists && (
          <span className="muted small chat-model">
            {c.model}
            {c.effort ? `, ${c.effort} effort` : ""}
          </span>
        )}
        <span className="chat-header-actions">
          {exists && (
            <Button type="button" disabled={!c.resume_command} onClick={copy}>
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
      {c ? <ChatMessages conversation={c} /> : <p className="muted small chat-note">Loading the Conversation…</p>}
      {exists && c.messages.length === 0 && !picking && <p className="muted small chat-note">Ask Claude about {p.project}.</p>}
      {!exists && c && !picking && <p className="muted small chat-note">Ask Claude about {p.project}: your first message starts a Conversation.</p>}
      {p.error && (
        <p role="alert" className="chat-refusal">
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

function unavailableReason(c: Conversation): string {
  return c.factory
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
