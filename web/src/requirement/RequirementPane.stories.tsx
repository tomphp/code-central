// The Requirement pane on the WASM Core: each story reads one Requirement back
// as the view does. Loading and the read's error are what SpecPaneNote shows
// in the pane's place.
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { Api } from "../api/api";
import { storyWorld } from "../api/core/story";
import type { SpecDetail } from "../api/types";
import { SpecPaneNote } from "../spec/SpecPaneNote";
import { RequirementPane } from "./RequirementPane";

const meta = { title: "Requirement/RequirementPane" } satisfies Meta;
export default meta;
type Story = StoryObj<typeof meta>;

type Requirement = SpecDetail & { requirement: NonNullable<SpecDetail["requirement"]> };
interface Loaded {
  api: Api;
  now: Date;
  spec: Requirement;
}

const pane = (build: (api: Api, s: Parameters<Parameters<typeof storyWorld>[0]>[0]) => Promise<string>): Story => ({
  ...storyWorld(async (s, w): Promise<Loaded> => {
    await s.project("acme", "Acme");
    const spec = await w.api.getSpec("acme", await build(w.api, s));
    return { api: w.api, now: w.now(), spec: { ...spec, requirement: spec.requirement! } };
  }),
  render: (_, { loaded }) => {
    const { api, now, spec } = loaded.world as Loaded;
    return <RequirementPane spec={spec} now={now} select={() => {}} specLink={(id) => ({ href: `#${id}`, onClick: () => {} })} />;
  },
});

export const Loading: Story = { render: () => <SpecPaneNote /> };
export const ReadError: Story = { render: () => <SpecPaneNote error="acme-9 isn't a Requirement in acme." /> };

// Just submitted: Grilling is waiting for a Factory and there are no Tickets to build.
export const Empty = pane(async (api) => (await api.submitRequirement("acme", "Export a Spec as Markdown")).spec.id);

// With implementation Tickets under Build.
export const Populated = pane(async (api, s) => {
  const { spec } = await api.submitRequirement("acme", "Export a Spec as Markdown");
  await s.ticket("acme", { title: "Render the Spec", spec: spec.id, state: "Accepted" });
  await s.ticket("acme", { title: "Add the export button", spec: spec.id, state: "In Review" });
  await s.ticket("acme", { title: "Document the format", spec: spec.id });
  return spec.id;
});
