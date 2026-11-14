// The Chat Messages of a Conversation (Spec code-central-633): the human's
// as plain text, Claude's as Markdown, each tool call as one collapsed line
// that opens to its result, permission denials as a warning, and how a
// reply ended. It follows the newest item while scrolled to the bottom.
import { useLayoutEffect, useRef } from "react";
import type { ChatItem, ChatMessage, Conversation } from "../api/types";
import { Markdown } from "../Markdown";

// AT_BOTTOM is how close to the bottom, in pixels, still counts as at it.
const AT_BOTTOM = 8;

export const TAIL_NOTICE = "Older messages were dropped";

export function ChatMessages({ conversation }: { conversation: Conversation }) {
  const box = useRef<HTMLDivElement>(null);
  const following = useRef(true);
  useLayoutEffect(() => {
    const el = box.current;
    if (el && following.current) el.scrollTop = el.scrollHeight;
  }, [conversation.messages]);
  const onScroll = () => {
    const el = box.current!;
    following.current = el.scrollHeight - el.scrollTop - el.clientHeight <= AT_BOTTOM;
  };
  return (
    <div role="log" aria-label="Conversation" className="chat-log" ref={box} onScroll={onScroll}>
      {conversation.tail && <p className="muted small chat-note">{TAIL_NOTICE}</p>}
      {conversation.messages.map((m, i) => (
        <Message key={i} message={m} />
      ))}
    </div>
  );
}

function Message({ message: m }: { message: ChatMessage }) {
  if (m.from === "human") {
    return (
      <div className="chat-message chat-human">
        <p>{m.text}</p>
      </div>
    );
  }
  const items = m.items ?? [];
  // A result shows under its call; one whose call wasn't held shows on its own.
  const calls = new Set(items.filter((i) => i.kind === "tool_call" && i.tool_use_id).map((i) => i.tool_use_id));
  const results = new Map(items.filter((i) => i.kind === "tool_result" && i.tool_use_id).map((i) => [i.tool_use_id, i]));
  return (
    <div className="chat-message chat-claude" aria-busy={!m.outcome}>
      {items.map((item) => {
        if (item.kind === "tool_result" && item.tool_use_id && calls.has(item.tool_use_id)) return null;
        return <Item key={item.seq} item={item} result={item.tool_use_id ? results.get(item.tool_use_id) : undefined} />;
      })}
      {!m.outcome && items.length === 0 && <p className="muted small chat-working">Claude is working on it…</p>}
      {m.outcome === "stopped" && <p className="muted chat-ending chat-stopped">Stopped</p>}
      {m.outcome === "failed" && (
        <p role="alert" className="chat-ending chat-failed">
          Error: {m.error || "the reply failed"}
        </p>
      )}
    </div>
  );
}

function Item({ item, result }: { item: ChatItem; result?: ChatItem }) {
  switch (item.kind) {
    case "text":
      return <Markdown>{item.text}</Markdown>;
    case "tool_call":
      return <ToolLine label={item.text} result={result} />;
    case "tool_result":
      return <ToolLine label={`${item.tool ?? "Tool"} result`} result={item} />;
    case "permission_denials":
      return (
        <p role="alert" className="chat-warning">
          <strong>Permission denied.</strong> {item.text}
        </p>
      );
    default:
      return <p className="muted small chat-log-line">{item.text}</p>;
  }
}

// ToolLine is a tool call as one line, collapsed until clicked, marked when
// its result failed or was cut.
function ToolLine({ label, result }: { label: string; result?: ChatItem }) {
  return (
    <details className="chat-tool">
      <summary>
        <code>{label}</code>
        {result?.is_error && <span className="chat-mark chat-error">error</span>}
        {result?.truncated && <span className="chat-mark">truncated</span>}
      </summary>
      {result ? <pre>{result.text}</pre> : <p className="muted small">No result yet.</p>}
    </details>
  );
}
