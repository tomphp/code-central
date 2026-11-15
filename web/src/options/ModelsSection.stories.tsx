// On the WASM Core: a Factory has reported what its claude accepts, and
// the section reads the Project back as the service sends it. Save writes
// to the Core, and a refused one shows Core's own refusal.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { storyWorld } from "../api/core/story";
import { ModelsSection } from "./ModelsSection";

// No component: the world gives its props, so the story takes no args.
const meta = {
  title: "Options/ModelsSection",
  ...storyWorld(async (s, w) => {
    await s.project("acme", "Acme");
    await s.reportModels("acme", { effort_levels: ["low", "medium", "high"], aliases: ["sonnet", "opus"], model_flag: "--model", effort_flag: "--effort" });
    return { project: (await w.api.listProjects())[0] };
  }),
  render: (_args, { loaded }) => <ModelsSection {...loaded.world} />,
} satisfies Meta;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
