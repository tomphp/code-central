// The free-text popups, each opened from its button. On the WASM Core:
// the Projects acme and beta, so New requirement, New Map and New Small
// Change ask which, and a submit writes to the Core. The api is passed
// because each popup submits; the Projects come from the world as props.
import type { ReactNode } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { Api } from "../api/api";
import { storyWorld } from "../api/core/story";
import type { ProjectRow } from "../api/types";
import { NewMap, NewPrototype, NewRequirement, NewRetro, NewSmallChange, WriteTheSpec } from "./NewRequirement";

interface World {
  api: Api;
  projects: ProjectRow[];
  spec: string;
  done: string;
}

const meta = {
  title: "Requirement/NewRequirement",
  ...storyWorld(async (s, w): Promise<World> => {
    await s.project("acme", "Acme");
    await s.project("beta", "Beta");
    const spec = await s.spec("acme", { title: "Sign-in" });
    const done = await s.ticket("acme", { title: "Add a sign-in page", spec, state: "Done" });
    return { api: w.api, projects: await w.api.listProjects(), spec, done };
  }),
} satisfies Meta;
export default meta;
type Story = StoryObj<typeof meta>;

// popup renders the button draw gives and opens its popup by clicking it.
const popup = (button: string, draw: (w: World) => ReactNode): Story => ({
  render: (_, { loaded }) => <>{draw(loaded.world as World)}</>,
  play: async ({ canvasElement }) => {
    await userEvent.click(within(canvasElement).getByRole("button", { name: button }));
  },
});

const submitted = () => {};

export const Requirement = popup("New requirement", ({ api, projects }) => <NewRequirement api={api} projects={projects} onSubmitted={submitted} />);

// With a Project filtered, it doesn't ask which.
export const RequirementInOneProject = popup("New requirement", ({ api, projects }) => (
  <NewRequirement api={api} projects={projects} project="acme" onSubmitted={submitted} />
));

// Named so as not to hide the global Map.
export const MapStory: Story = { ...popup("New Map", ({ api, projects }) => <NewMap api={api} projects={projects} onSubmitted={submitted} />), name: "Map" };

export const SmallChange = popup("New Small Change", ({ api, projects }) => <NewSmallChange api={api} projects={projects} onSubmitted={submitted} />);

export const Prototype = popup("New Prototype", ({ api, spec }) => <NewPrototype api={api} spec={spec} onSubmitted={submitted} />);

// Prefilled from the finished Map.
export const WriteTheSpecFromAMap = popup("Write the Spec", ({ api }) => (
  <WriteTheSpec
    api={api}
    map={{ id: "acme-9", title: "Sign-in everywhere", destination: "Every app signs in through one service, with GitHub and Google." }}
    onSubmitted={submitted}
  />
));

export const Retro = popup("Retro", ({ api, done }) => <NewRetro api={api} target={done} onSubmitted={submitted} />);
