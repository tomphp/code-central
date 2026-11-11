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

An end-to-end test (`web/src/e2e/heartbeat.e2e.test.ts`) sends these same bodies to a real service, so the example stays true. Read the result in any Project view: `factory_status` in `get_project` and `list_projects` holds the combined `status` and each Factory's `name`, `host`, `status`, `note`, `last_heartbeat` and `interval_seconds`.
