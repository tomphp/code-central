import type { Meta, StoryObj } from "@storybook/react-vite";
import { CIRunEntry } from "./CIRunEntry";
import { NOW, finished, running } from "./ciFixtures";

const meta = { title: "CI/CIRunEntry" } satisfies Meta;
export default meta;
type Story = StoryObj<typeof meta>;

const navigate = () => {};

// Rows in the CI view's table, in All Projects: a running run, a failed
// re-run tracked against a Ticket, and an ignored one tracked against a
// CI Rule.
export const Row: Story = {
  render: () => (
    <table className="data-table">
      <tbody>
        <CIRunEntry layout="row" run={running("acme", 3)} now={NOW} projectName="Acme" navigate={navigate} />
        <CIRunEntry layout="row" run={finished("acme", 2, { conclusion: "failure", attempt: 2, ticket: "acme-1" })} now={NOW} projectName="Acme" navigate={navigate} />
        <CIRunEntry
          layout="row"
          run={finished("beta", 1, { ignored: true, workflow: "nightly.yml", name: "Nightly", rule: "beta-r1" })}
          provider="azure_pipelines"
          now={NOW}
          projectName="Beta"
          navigate={navigate}
        />
      </tbody>
    </table>
  ),
};

// Items in a Ticket's CI section: a failed run with its jobs, a running
// one, and an ignored one.
export const Item: Story = {
  render: () => (
    <ul className="ci-runs">
      <CIRunEntry
        layout="item"
        run={finished("acme", 1, {
          conclusion: "failure",
          attempt: 2,
          jobs: [
            { name: "test", conclusion: "failure", url: "https://example.com/job/1" },
            { name: "lint", conclusion: "success", url: "https://example.com/job/2" },
          ],
        })}
      />
      <CIRunEntry layout="item" run={running("acme", 2)} />
      <CIRunEntry layout="item" run={finished("acme", 3, { ignored: true, workflow: "nightly.yml", name: "Nightly" })} />
    </ul>
  ),
};

// Cards on the Activity Dashboard, in All Projects: a running run and a
// queued one not started yet.
export const Card: Story = {
  render: () => (
    <ul className="dashboard-list">
      <CIRunEntry layout="card" run={{ ...running("acme", 1), ticket: "acme-1" }} now={NOW} projectName="Acme" to={{ view: "inbox", ticket: "acme-1" }} navigate={navigate} />
      <CIRunEntry
        layout="card"
        run={{ ...running("beta", 2), status: "queued", started: undefined, ticket: "beta-4" }}
        provider="azure_pipelines"
        now={NOW}
        projectName="Beta"
        to={{ view: "inbox", ticket: "beta-4" }}
        navigate={navigate}
      />
    </ul>
  ),
};
