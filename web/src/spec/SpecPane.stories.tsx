// The plain Spec pane on the WASM Core: each story reads one Spec back as the
// view does. Loading and the read's error are what SpecPaneNote shows in the
// pane's place.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { storyWorld } from "../api/core/story";
import type { SpecDetail } from "../api/types";
import { SpecPane } from "./SpecPane";
import { SpecPaneNote } from "./SpecPaneNote";

const meta = { title: "Spec/SpecPane" } satisfies Meta;
export default meta;
type Story = StoryObj<typeof meta>;

interface Loaded {
  now: Date;
  spec: SpecDetail;
}

const pane = (build: (s: Parameters<Parameters<typeof storyWorld>[0]>[0]) => Promise<string>, archived = false): Story => ({
  ...storyWorld(async (s, w): Promise<Loaded> => {
    await s.project("acme", "Acme");
    return { now: w.now(), spec: await w.api.getSpec("acme", await build(s)) };
  }),
  render: (_, { loaded }) => {
    const { now, spec } = loaded.world as Loaded;
    return <SpecPane spec={spec} archived={archived} now={now} select={() => {}} />;
  },
});

export const Loading: Story = { render: () => <SpecPaneNote /> };
export const ReadError: Story = { render: () => <SpecPaneNote error="acme-9 isn't a Spec in acme." /> };

// A Spec with no Tickets yet.
export const Empty = pane((s) => s.spec("acme", { title: "Sign-in", body: "Let people sign in with a password." }));

export const Populated = pane(async (s) => {
  const spec = await s.spec("acme", { title: "Sign-in", body: "Let people sign in with a password." });
  await s.ticket("acme", { title: "Add a sign-in page", spec, state: "Accepted" });
  await s.ticket("acme", { title: "Rate-limit the endpoint", spec, state: "In Review" });
  await s.ticket("acme", { title: "Store sessions server-side", spec });
  return spec;
});

// An Archived Project's Spec takes no Retro.
export const Archived = pane(async (s) => {
  const spec = await s.spec("acme", { title: "Sign-in" });
  await s.ticket("acme", { title: "Add a sign-in page", spec, state: "Accepted" });
  return spec;
}, true);
