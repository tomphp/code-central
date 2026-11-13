// Focus Mode: one Focus Item at a time from Needs Me, filling the window
// with no nav, list or side pane. A layout only: the data comes from
// live.ts, as the Inbox's does, and focusQueue picks what to show. Its
// root's class scopes the Focus styles, so the panels inside read larger
// here and nowhere else.
//
// The panels and the Question form get an Api whose writes (act, and
// unlink) first record that the item on screen was acted on, so an item that leaves after the
// human's own action moves on silently, and one that leaves otherwise
// moves on with a brief "Dealt with elsewhere" note. Submitting from
// + New ▾ shows a brief "Requirement added" or "Map added" note instead;
// there's only ever one note, the latest.
import { useEffect, useMemo, useState } from "react";
import type { Api } from "../api/api";
import type { FocusClear } from "../api/types";
import { ticketOf } from "../format";
import { useTicket, type InboxData } from "../live";
import { NewSplit } from "../requirement/NewSplit";
import type { UiLocation } from "../url";
import { ActivityDashboard } from "./ActivityDashboard";
import { FocusItemView, inboxOf } from "./FocusItemView";
import { focusQueue, type FocusItem } from "./queue";
import { Button } from "../controls/Button";
import "./focus.css";

export interface FocusProps {
  api: Api;
  now: Date;
  version: number;
  data?: InboxData;
  // focusClear is what to show when nothing needs the human: All clear,
  // or the Activity Dashboard.
  focusClear: FocusClear;
  loc: UiLocation;
  navigate(patch: Partial<UiLocation>): void;
}

export function Focus({
  api,
  now,
  version,
  data,
  focusClear,
  loc,
  navigate,
}: FocusProps) {
  const project = data?.projects.find((p) => p.key === loc.project);
  const [onScreen, setOnScreen] = useState<FocusItem>();
  // skipped lives here for this visit only: leaving Focus Mode or
  // reloading forgets it, and it's never sent or put in the URL.
  const [skipped, setSkipped] = useState<FocusItem[]>([]);
  // actedOn is the key of the item on screen once the human acts on it
  // here, until a refusal takes it back.
  const [actedOn, setActedOn] = useState<string>();
  const [note, setNote] = useState<Note>();
  // A CI item has no Ticket of its own: its panel shows the first Ticket
  // it lists, whose options act on every one of them.
  const ticket = useTicket(
    api,
    onScreen?.row.project,
    onScreen && ticketOf(onScreen.row),
    version,
  );
  const acted = !!onScreen && actedOn === onScreen.key;
  const screen =
    data &&
    focusQueue({
      needsMe: data.needsMe,
      project: loc.project,
      onScreen,
      ticket: ticket.data,
      skipped,
      acted,
    });
  const item = screen?.item;
  // What's shown becomes what's on screen, so a later read keeps it there.
  // Moving on shows "Dealt with elsewhere" only when the item left without
  // the human, and only over a next item, so All clear stays calm; moving
  // on otherwise ends it, but not an "added" note. A Round becoming its
  // first Question, once read, isn't moving on, so it keeps the note.
  useEffect(() => {
    if (!screen || item?.key === onScreen?.key) return;
    const roundRead =
      !!onScreen?.roundKey &&
      !onScreen.question &&
      onScreen.roundKey === item?.roundKey;
    setOnScreen(item);
    setActedOn(undefined);
    if (roundRead) return;
    if (screen.elsewhere && item)
      setNote({ text: "Dealt with elsewhere", elsewhere: true });
    else setNote((n) => (n?.elsewhere ? undefined : n));
  }, [screen, item, onScreen]);
  // The note is brief: it goes after NOTE_MS, and a newer note starts its
  // own NOTE_MS.
  useEffect(() => {
    if (!note) return;
    const timer = setTimeout(() => setNote(undefined), NOTE_MS);
    return () => clearTimeout(timer);
  }, [note]);
  const itemKey = item?.key;
  const itemApi = useMemo(
    () =>
      itemKey
        ? recordingWrites(api, {
            onWrite: () => setActedOn(itemKey),
            onRefused: () => setActedOn((k) => (k === itemKey ? undefined : k)),
          })
        : undefined,
    [api, itemKey],
  );

  return (
    <main aria-label="Focus Mode" className="focus">
      <header className="focus-head">
        {loc.project && (
          <span className="focus-project">
            {project?.display_name ?? loc.project}{" "}
            <span className="muted">{loc.project}</span>
          </span>
        )}
        {screen && !screen.clear && (
          <span className="focus-left">{screen.remaining} left</span>
        )}
        {/* Submitting goes nowhere: whatever is on screen stays, skips and
            all, with a note on every screen. */}
        <div className="focus-new">
          <NewSplit
            variant="plain"
            api={api}
            projects={data?.projects.filter((p) => !p.archived) ?? []}
            project={loc.project}
            onSubmitted={({ kind }) => setNote({ text: `${kind} added` })}
          />
        </div>
        <Button  onClick={() => navigate({ view: "inbox" })}>
          Exit
        </Button>
      </header>
      {note && (
        <p role="status" className="focus-note">
          {note.text}
        </p>
      )}
      {!screen ? (
        <p className="focus-loading">Loading…</p>
      ) : screen.clear && focusClear === "activity_dashboard" ? (
        <ActivityDashboard
          api={api}
          now={now}
          version={version}
          data={data}
          loc={loc}
          navigate={navigate}
        />
      ) : screen.clear ? (
        <section aria-label="All clear" className="focus-clear">
          <h2>All clear</h2>
          <p>Nothing needs you.</p>
        </section>
      ) : screen.onlySkipped ? (
        <section
          aria-label="Only skipped Focus Items left"
          className="focus-clear"
        >
          <p>You skipped {screen.remaining}, go through them again?</p>
          <Button variant="primary" onClick={() => setSkipped([])}>
            Go through them again
          </Button>
        </section>
      ) : item && itemApi ? (
        <FocusItemView
          key={item.key}
          api={itemApi}
          now={now}
          data={data}
          item={item}
          ticket={ticket.data?.id === ticketOf(item.row) ? ticket.data : undefined}
          navigate={navigate}
          skip={() => setSkipped([...skipped, item])}
          inbox={inboxOf(item.row, loc.project)}
        />
      ) : (
        <p className="focus-loading">Loading…</p>
      )}
    </main>
  );
}

// Note is the brief note under the header; elsewhere marks "Dealt with
// elsewhere", which moving on ends.
interface Note {
  text: string;
  elsewhere?: boolean;
}

// NOTE_MS is how long a note stays.
const NOTE_MS = 4000;

// WRITES are the Api calls a panel or the Question form makes to act on
// a Ticket.
const WRITES: ReadonlySet<PropertyKey> = new Set<keyof Api>(["act", "unlink"]);

// recordingWrites is api with each write calling onWrite before it's
// forwarded, so the live notice it causes finds it recorded, and
// onRefused if it's refused, as a refused write left the item where it
// was. Every other call goes straight to api.
function recordingWrites(
  api: Api,
  { onWrite, onRefused }: { onWrite(): void; onRefused(): void },
): Api {
  return new Proxy(api, {
    get(target, prop) {
      const v = Reflect.get(target, prop);
      if (typeof v !== "function") return v;
      if (!WRITES.has(prop)) return v.bind(target);
      return (...args: unknown[]) => {
        onWrite();
        return (v.apply(target, args) as Promise<unknown>).catch(
          (e: unknown) => {
            onRefused();
            throw e;
          },
        );
      };
    },
  });
}
