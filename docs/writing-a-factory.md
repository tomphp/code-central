# Writing a Factory

A Factory is any program that claims a Project's Tickets and works them, as [`code-central-factory`](running-the-factory.md) does. This guide covers the one thing Code Central asks of it beyond the Tickets: a **Heartbeat**, so the Project's dot in the left pane says whether anything is working it. A Factory that sends none works fine; its Project just shows Offline.

Code Central never probes a Factory and never guesses one from its other calls. The Factory announces itself, and Code Central forgets every Heartbeat when it restarts, so each Factory's next Heartbeat brings it back.

## The call

```text
POST /api/p/{key}/report_heartbeat
Content-Type: application/json
```

`{key}` is the Project's key, such as `acme-shop`. The body:

| Field | |
| --- | --- |
| `factory` | Required. The Factory's name, such as `code-central-factory` or `sandcastle`. |
| `host` | Required. The machine it runs on. With `factory`, it identifies the Factory: a new Heartbeat replaces that Factory's last one. |
| `state` | Required. `online`, `draining` (claiming nothing new while it finishes running turns) or `stopping` (it is exiting). |
| `note` | Optional. Free text for the human, cut to 200 characters. Code Central never interprets it. |
| `interval_seconds` | Required. How often the Factory polls, from 1 to 86400. It sets when Code Central gives up on it, below. |
| `chat` | Optional boolean, `false` when left out. **Chat support**: the Factory can run Conversations for the Project's Chat. Chat is available for the Project while an Online Factory reports it. A change in it counts as a change, so the UI refetches. |
| `plan_limits` | Optional. The account's **Plan Limits** as the Factory last read them; see [Plan Limits](#plan-limits). |
| `full` | Optional. Returns the Project in place of the acknowledgement. |

```console
$ curl -s -X POST http://localhost:7733/api/p/acme-shop/report_heartbeat \
    -H 'Content-Type: application/json' \
    -d '{"factory":"sandcastle","host":"build-1","state":"online","note":"2 of 3 turns running","interval_seconds":30}'
```

Invalid input is refused with the field named, an unknown Project is not found, and an archived Project refuses it like every write.

## Who may call it

There is no token. The request's `Host` header must be `localhost`, `127.0.0.1`, `[::1]` or `host.docker.internal` (the last for a Factory in a container, with `CODE_CENTRAL_URL=http://host.docker.internal:7733`), and it must carry no cross-origin `Origin` header. Any other Host is refused with 403. A script or a `fetch` in Node sends neither a foreign Host nor an Origin, so it passes.

## When a Factory is Offline

Code Central works the status out on every read:

- **Offline** if the last Heartbeat's state was `stopping`, at once, or if the time since the last Heartbeat is at least the larger of 3 × its `interval_seconds` and 90 seconds. A crash or `kill -9` sends nothing, so it shows Offline at that deadline, within about 10 seconds.
- otherwise **Draining** if the last state was `draining`;
- otherwise **Online**.

The Project's dot is Online if any Factory is Online, else Draining if any is, else Offline. A stopped Factory keeps its note, so the tooltip still says why it stopped.

## Plan Limits

A Factory that runs `claude` can report the Claude account's **Plan Limits**, its 5-hour limit and its weekly limit, as it last read them from its Turns. Send them in `plan_limits` on every Heartbeat:

| Field | |
| --- | --- |
| `observed_at` | Required. When the Factory read the figures, an RFC 3339 time. A time later than Code Central's receipt of the Heartbeat is kept as the receipt time, so clock skew never makes figures look newer than they are; the same figures sent again keep that first receipt time. |
| `five_hour` | Optional. The 5-hour window: `used_percent`, from 0 to 100, and `resets_at`, an RFC 3339 time, both required. |
| `weekly` | Optional. The weekly window, as `five_hour`. |

Leave out a window the Factory has no figures for. `plan_limits` with neither window counts as none. A `used_percent` outside 0 to 100, a window without `resets_at` or `plan_limits` without `observed_at` is refused with the field named, such as `plan_limits.weekly.used_percent`.

Each Heartbeat replaces the Factory's last one whole, so keep sending the figures you hold: a Heartbeat without `plan_limits` says the Factory has none. Like the Heartbeat, Code Central keeps them in memory only.

```console
$ curl -s -X POST http://localhost:7733/api/p/acme-shop/report_heartbeat \
    -H 'Content-Type: application/json' \
    -d '{"factory":"sandcastle","host":"build-1","state":"online","interval_seconds":30,
         "plan_limits":{"observed_at":"2026-11-16T09:00:00Z",
                        "five_hour":{"used_percent":42,"resets_at":"2026-11-16T12:00:00Z"},
                        "weekly":{"used_percent":7,"resets_at":"2026-11-20T09:00:00Z"}}}'
```

The Project's `factory_status` carries `plan_limits`, in the same shape: the figures with the latest `observed_at` among its Factories that aren't Offline, left out while none has any. A Factory going Offline drops its figures out at once. New or changed figures count as a change, so the UI refetches; the same figures again don't.

## Stop and Resume

Name your Factory on `claim` (the `factory` field), so Code Central knows which Factory holds the Claim. A human can Stop a live Claim, which marks it Stopped. For a Stopped Claim, stop its Turn, keep the Claim, and don't resume it until it is no longer Stopped; it takes no slot. When the human Resumes it, resume the same session.

## Waking the poll

A Factory polls for work on its own timer. It can also ask Code Central to say when a poll is worth running, so work is picked up at once instead of on the next tick. This is optional: a Factory that doesn't call it just polls, and Code Central loses nothing.

### `next_factory_wake`

```text
GET /api/p/{key}/factory/next-wake?factory=sandcastle&host=build-1
```

`factory` and `host` are as in the Heartbeat, and are the only parameters. The call holds for at most Code Central's wait timeout, 60 seconds, then answers JSON:

| Field | |
| --- | --- |
| `woken` | `true` if something happened that is worth a poll. `false` if the wait timed out, a newer call replaced this one or the request ended. |

The answer carries no work: on `woken`, run your usual poll. Then call again at once, whatever the answer. Keep one open at a time; a newer call from the same Factory and host replaces the older, which answers `woken: false`. On the same terms as the Heartbeat (see Who may call it).

A wake comes when a write that a poll acts on happens on the Project:

- a Ticket moves into Ready, Answered, Accepted, Done or Cancelled;
- a live Claim ends by a move its holder didn't make (cancel, send back, defer, release or a move by hand);
- a Ticket's Blockers shrink;
- a Project setting the Factory reads changes (the base, Landing, CI, Models, Stage Guides, Auto-accept or Archived);
- an Import lands;
- a Map's `guided` Tag is turned on;
- a live Claim's Stopped mark changes (a human's Stop or Resume).

Comments, Session Events, Usage, Rounds being posted, `finish` to In Review, `claim`, CI Runs, Chat and Options don't wake it. A change between two calls can be missed, so keep polling on your timer as the fallback; a missed wake costs at most one interval. A Code Central too old to have the operation answers not found: stop calling it and poll only.

## Running Chat turns

A Factory that reports `chat: true` is expected to answer the Project's Conversation. Chat takes two calls, on the same terms as the Heartbeat (see Who may call it). Code Central never calls the Factory.

### `next_chat_work`

```text
GET /api/p/{key}/chat/next-work?factory=sandcastle&host=build-1
```

`factory` and `host` are as in the Heartbeat. The call holds until there is work for this Factory, for at most Code Central's wait timeout, then answers. Keep one open at a time; a newer call from the same Factory replaces the older. The answer is JSON holding one of:

| Field | |
| --- | --- |
| `turn` | One Chat Message to answer: `conversation` (its ID), `text`, `model`, `effort` (empty for Claude Code's default), `first` (true when it starts the Conversation's session) and, when not `first`, the `session_id` to resume. |
| `stop` | `{"conversation": "<id>"}`: stop that Conversation's running reply. |
| *(neither)* | The wait timed out. Call again. |

Code Central hands out the next turn only after the reply before it has ended.

### `post_chat_events`

```text
POST /api/p/{key}/chat/events
Content-Type: application/json
```

The Factory that took the turn posts the reply as it arrives, as often as it likes:

| Field | |
| --- | --- |
| `conversation` | Required. The turn's `conversation`. |
| `factory`, `host` | Required. As in the Heartbeat; they must name the Factory holding the Conversation. |
| `session_id`, `resume_command` | On the first post: the session, and the command that resumes it in the Factory's worktree. |
| `items` | The reply's new items, in the shape of Session Events (`time`, `kind`, `text`, and for tools `tool`, `tool_use_id`, `is_error`). The pop-up streams them. |
| `outcome` | On the last post only: `done`, `stopped` or `failed`. Leave it out while the reply runs. |
| `error` | With `failed`: the text the pop-up shows. |

It always answers 204, even for a post it drops, so a Factory never fails a turn on it. A reply whose Factory goes Offline ends with `the Factory stopped answering`. Conversations live in memory only, so a Code Central restart drops them.

```console
$ curl -s -X POST http://localhost:7733/api/p/acme-shop/chat/events \
    -H 'Content-Type: application/json' \
    -d '{"conversation":"<id>","factory":"sandcastle","host":"build-1","items":[{"time":"2026-11-14T10:00:00Z","kind":"text","text":"Hello"}],"outcome":"done"}'
```

An end-to-end test (`web/src/e2e/chat.e2e.test.ts`) runs this exchange against a real service.

## An example

Send a Heartbeat on each poll, and a `stopping` one on the way out:

```ts
const base = "http://localhost:7733";
const project = "acme-shop";
const intervalSeconds = 30;

async function heartbeat(state: "online" | "draining" | "stopping", note: string) {
  try {
    await fetch(`${base}/api/p/${project}/report_heartbeat`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        factory: "sandcastle",
        host: (await import("node:os")).hostname(),
        state,
        note,
        interval_seconds: intervalSeconds,
      }),
    });
  } catch (err) {
    console.error("heartbeat failed:", err); // never let it stop the poll
  }
}

await heartbeat("online", "starting");
const timer = setInterval(() => heartbeat("online", "polling"), intervalSeconds * 1000);

for (const signal of ["SIGINT", "SIGTERM"] as const) {
  process.on(signal, async () => {
    clearInterval(timer);
    await heartbeat("stopping", `stopped on ${signal}`);
    process.exit(0);
  });
}
```

An end-to-end test (`web/src/e2e/heartbeat.e2e.test.ts`) sends these same bodies to a real service, so the example stays true. Read the result in any Project view: `factory_status` in `get_project` and `list_projects` holds the combined `status` and each Factory's `name`, `host`, `status`, `note`, `last_heartbeat`, `interval_seconds` and `chat`, and `plan_limits` when there are any.
