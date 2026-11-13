import type { Meta, StoryObj } from "@storybook/react-vite";
import { CISection } from "./CISection";
import { storyApi, storyProject } from "./storyFixtures";

const meta = {
  title: "Options/CISection",
  component: CISection,
  args: { api: storyApi(), project: storyProject() },
} satisfies Meta<typeof CISection>;
export default meta;
type Story = StoryObj<typeof meta>;

export const GitHubActions: Story = {};
export const AzurePipelines: Story = {
  args: {
    project: storyProject({
      ci: {
        ...storyProject().ci,
        provider: "azure_pipelines",
        project_url: "https://dev.azure.com/acme/web",
        rules: [],
      },
    }),
  },
};
export const TrackingOff: Story = {
  args: {
    project: storyProject({ ci: { ...storyProject().ci, tracking: false } }),
  },
};
