import type { Meta, StoryObj } from "@storybook/react-vite";
import { CIBody } from "./CIView";
import { NOW, finished, projectName, running, status } from "./ciFixtures";

const meta = {
  title: "CI/CIBody",
  parameters: { layout: "fullscreen" },
  args: { now: NOW, projectName, navigate: () => {} },
  render: (args) => (
    <main className="ci view-shell">
      <CIBody {...args} />
    </main>
  ),
} satisfies Meta<typeof CIBody>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Loading: Story = {};
export const Empty: Story = { args: { project: "acme", overview: { projects: [status("acme")], runs: [] } } };
export const Populated: Story = {
  args: {
    project: "acme",
    overview: {
      projects: [status("acme")],
      runs: [
        running("acme", 3),
        finished("acme", 2, { conclusion: "failure", ticket: "acme-1" }),
        finished("acme", 1, { ignored: true, workflow: "nightly.yml", name: "Nightly" }),
      ],
    },
  },
};
export const ErrorState: Story = { args: { project: "acme", error: "The CI view couldn't be read." } };
export const OneProject: Story = {
  args: { project: "beta", overview: { projects: [status("beta", { watcher: { state: "error", message: "gh isn't logged in.", fix: "Run gh auth login." } })], runs: [finished("beta", 7)] } },
};
export const ManyProjects: Story = {
  args: {
    overview: {
      projects: [status("acme"), status("beta", { tracking: false })],
      runs: [running("acme", 3), finished("acme", 4, { rule: "acme-r1" })],
    },
  },
};
