import type { Meta, StoryObj } from "@storybook/react-vite";
import { CISection } from "./CISection";
import { optionsWorld } from "./storyFixtures";

const meta = {
  title: "Options/CISection",
  render: (_args, { loaded }) => <CISection {...loaded.world} />,
} satisfies Meta;
export default meta;
type Story = StoryObj<typeof meta>;

export const GitHubActions: Story = {
  ...optionsWorld(async (s, w) => {
    await s.reportPushRemote("acme", "git@github.com:acme/web.git");
    await w.api.setCITracking("acme", { tracking: true, provider: "github_actions", repository: "acme/web", ignored_workflows: ["nightly.yml"] });
  }),
};
export const AzurePipelines: Story = {
  ...optionsWorld(async (_s, w) => {
    await w.api.setCITracking("acme", { tracking: true, provider: "azure_pipelines", repository: "", project_url: "https://dev.azure.com/acme/web", ignored_workflows: [] });
  }),
};
export const TrackingOff: Story = { ...optionsWorld() };
