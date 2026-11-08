// The Live section under a Claim in the Ticket detail (ticket 111): a
// disclosure that, while open, follows the Claim's Transcript. Agent text
// is Markdown, a tool call one line with its result collapsed beneath it,
// and the Factory's own lines are set apart. It follows the newest event
// while scrolled to the bottom.
import { useEffect, useLayoutEffect, useRef, useState } from "react";
import type { Api } from "../api/api";
import type { HeldSessionEvent } from "../api/types";
import { Markdown } from "../Markdown";

// AT_BOTTOM is how close to the bottom, in pixels, still counts as at it.
const AT_BOTTOM = 8;

export interface LiveProps {
  api: Api;
  project: string;
  id: string;
  claim: number;
  // defaultOpen is whether it should be open: while the Claim is live and
  // the Ticket In Progress. It opens whenever this turns true, and is
  // otherwise left as the human set it.
  defaultOpen: boolean;
}

export function Live({ api, project, id, claim, defaultOpen }: LiveProps) {
  const [open, setOpen] = useState(defaultOpen);
  useEffect(() => {
    if (defaultOpen) setOpen(true);
  }, [defaultOpen]);
  return (
    <details aria-label="Live" className="live" open={open} onToggle={(e) => setOpen(e.currentTarget.open)}>
      <summary>Live</summary>
      {open && <TranscriptLog api={api} project={project} id={id} claim={claim} />}
    </details>
  );
}

// TranscriptLog subscribes to the Claim's Transcript for as long as it is
// mounted. The Transcript read says whether there is none, since the
// stream can't say that nothing more is on its way.
function TranscriptLog({ api, project, id, claim }: Omit<LiveProps, "defaultOpen">) {
  const [events, setEvents] = useState<HeldSessionEvent[]>([]);
  const [tail, setTail] = useState(false);
  const [noneHeld, setNoneHeld] = useState(false);
  const box = useRef<HTMLDivElement>(null);
  const following = useRef(true);

  useEffect(() => {
    let current = true;
    api.transcript(project, id, claim).then(
      (tr) => current && setNoneHeld(tr.events.length === 0),
      () => {},
    );
    const stop = api.subscribeTranscript(project, id, claim, {
      // The stream never repeats one; this keeps a repeat from showing twice if it did.
      onEvent: (e) => setEvents((es) => (es.length && es[es.length - 1].seq >= e.seq ? es : [...es, e])),
      onTail: () => setTail(true),
    });
    return () => {
      current = false;
      stop();
    };
  }, [api, project, id, claim]);

  useLayoutEffect(() => {
    const el = box.current;
    if (el && following.current) el.scrollTop = el.scrollHeight;
  }, [events]);

  const onScroll = () => {
    const el = box.current!;
    following.current = el.scrollHeight - el.scrollTop - el.clientHeight <= AT_BOTTOM;
  };

  const results = new Map<string, HeldSessionEvent>();
  const calls = new Set<string>();
  for (const e of events) {
    if (e.kind === "tool_call" && e.tool_use_id) calls.add(e.tool_use_id);
    if (e.kind === "tool_result" && e.tool_use_id) results.set(e.tool_use_id, e);
  }

  return (
    <div role="log" aria-label="Transcript" className="transcript" ref={box} onScroll={onScroll}>
      {tail && (
        <p className="muted small transcript-note">
          Earlier events aren't held (Code Central keeps the last 2,000 and none from before its last restart)
        </p>
      )}
      {noneHeld && events.length === 0 && <p className="muted small transcript-note">This Claim has no Transcript.</p>}
      {events.map((e) => {
        // A result shows under its call; one whose call wasn't held shows on its own.
        if (e.kind === "tool_result" && e.tool_use_id && calls.has(e.tool_use_id)) return null;
        return <Row key={e.seq} event={e} result={e.tool_use_id ? results.get(e.tool_use_id) : undefined} />;
      })}
    </div>
  );
}

// Row is one Session Event; a tool call carries its result, if held.
function Row({ event: e, result }: { event: HeldSessionEvent; result?: HeldSessionEvent }) {
  switch (e.kind) {
    case "text":
      return (
        <div className="transcript-row transcript-text">
          <Time at={e.time} />
          <Markdown>{e.text}</Markdown>
        </div>
      );
    case "tool_call":
      return (
        <div className="transcript-row">
          <Time at={e.time} />
          <code className="transcript-call">{e.text}</code>
          {result && <ToolResult event={result} />}
        </div>
      );
    case "tool_result":
      return (
        <div className="transcript-row">
          <Time at={e.time} />
          <ToolResult event={e} />
        </div>
      );
    case "factory":
      return (
        <div className="transcript-row transcript-factory">
          <Time at={e.time} />
          <span className="transcript-marker">Factory</span> {e.text}
        </div>
      );
  }
}

// ToolResult is a tool call's result, collapsed until clicked, marked when
// it failed or was cut.
function ToolResult({ event: e }: { event: HeldSessionEvent }) {
  return (
    <details aria-label="Result" className="transcript-result">
      <summary>
        <Time at={e.time} /> result
        {e.is_error && <span className="transcript-mark transcript-error">error</span>}
        {e.truncated && <span className="transcript-mark">truncated</span>}
      </summary>
      <pre>{e.text}</pre>
    </details>
  );
}

// Time is a row's time of day; its dateTime is the full time.
function Time({ at }: { at: string }) {
  return (
    <time dateTime={at} className="muted small">
      {new Date(at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" })}
    </time>
  );
}
