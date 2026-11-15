// The Map body on the WASM Core: each story reads one Map back as the view
// does and gives MapBody the outcome of its actions as props. Loading and the
// read's error are what SpecPaneNote shows in the pane's place.
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { Api } from "../api/api";
import { storyWorld } from "../api/core/story";
import type { MapView, SpecDetail } from "../api/types";
import { SpecPaneNote } from "../spec/SpecPaneNote";
import { MapBody } from "./MapPane";

const meta = { title: "Map/MapBody" } satisfies Meta;
export default meta;
type Story = StoryObj<typeof meta>;

type Map = SpecDetail & { map: MapView };
interface Loaded {
  api: Api;
  now: Date;
  spec: Map;
}

const BODY = [
  "## Destination",
  "Work offline on a plane.",
  "## Decisions so far",
  "- Store drafts in IndexedDB.",
  "## Out of scope",
  "- Conflict resolution.",
  "## Not yet specified",
  "How a draft syncs once the plane lands.",
].join("\n\n");

const pane = (build: (api: Api, s: Parameters<Parameters<typeof storyWorld>[0]>[0]) => Promise<string>, extra: { busy?: boolean; error?: string } = {}): Story => ({
  ...storyWorld(async (s, w): Promise<Loaded> => {
    await s.project("acme", "Acme");
    const spec = await w.api.getSpec("acme", await build(w.api, s));
    return { api: w.api, now: w.now(), spec: { ...spec, map: spec.map! } };
  }),
  render: (_, { loaded }) => {
    const { api, now, spec } = loaded.world as Loaded;
    return (
      <MapBody
        spec={spec}
        now={now}
        busy={!!extra.busy}
        error={extra.error}
        select={() => {}}
        specLink={(id) => ({ href: `#${id}`, onClick: () => {} })}
        written={() => {}}
        start={() => {}}
        setGuided={() => {}}
      />
    );
  },
});

export const Loading: Story = { render: () => <SpecPaneNote /> };
export const ReadError: Story = { render: () => <SpecPaneNote error="acme-9 isn't a Map in acme." /> };

// Just submitted: only the Charting Ticket, and no Destination yet.
export const Empty = pane(async (api) => (await api.submitMap("acme", "Offline mode")).spec.id);

// Charted, with decisions made, one waiting to start and the sections written.
export const Populated = pane(async (api, s) => {
  const { spec, charting } = await api.submitMap("acme", "Offline mode");
  await s.accepted("acme", charting.id);
  await s.ticket("acme", { title: "Which store", spec: spec.id, state: "Accepted", labels: ["wayfinder:grilling"] });
  await s.ticket("acme", { title: "Try a cache", spec: spec.id, state: "In Review", labels: ["prototype"] });
  await s.ticket("acme", { title: "How to sync", spec: spec.id, state: "Draft", labels: ["wayfinder:grilling"] });
  await api.editSpec("acme", spec.id, { body: BODY });
  return spec.id;
});

// An action is under way, so Start and Guided are disabled.
export const Busy = pane(async (api) => (await api.submitMap("acme", "Offline mode", true)).spec.id, { busy: true });

// The last action failed.
export const ActionError = pane(async (api) => (await api.submitMap("acme", "Offline mode")).spec.id, { error: "acme-3 can't be approved yet." });
