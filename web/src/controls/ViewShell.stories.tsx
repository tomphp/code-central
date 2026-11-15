import type { Meta, StoryObj } from "@storybook/react-vite";

// The view shell, 1100px view body and data table are global classes
// (styles.css), not components; this story shows them together.
const meta = { title: "Controls/ViewShell", parameters: { layout: "fullscreen" } } satisfies Meta;
export default meta;
type Story = StoryObj<typeof meta>;

export const WithDataTable: Story = {
  render: () => (
    <main className="view-shell">
      <div className="view-body">
        <table aria-label="Example" className="data-table data-table-mono">
          <thead>
            <tr>
              <th scope="col">Name</th>
              <th scope="col" className="num">Count</th>
            </tr>
          </thead>
          <tbody>
            <tr>
              <th scope="row">list_tickets</th>
              <td className="num">12</td>
            </tr>
            <tr className="pulse-0">
              <th scope="row">get_ticket</th>
              <td className="num">3</td>
            </tr>
          </tbody>
        </table>
      </div>
    </main>
  ),
};
