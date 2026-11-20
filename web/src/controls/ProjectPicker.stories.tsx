import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { FactoryStatusName, ProjectRow } from "../api/types";
import { Button } from "./Button";
import { ProjectPicker } from "./ProjectPicker";
import { usePopover } from "./usePopover";

const meta = { title: "Controls/Project picker" } satisfies Meta;
export default meta;
type Story = StoryObj<typeof meta>;

const project = (key: string, display_name: string, status: FactoryStatusName, archived = false) =>
  ({ key, display_name, archived, factory_status: { status, factories: [] } }) as unknown as ProjectRow;

const projects = [
  project("web", "Web app", "online"),
  project("api", "api service", "draining"),
  project("docs", "Docs", "offline"),
  project("old", "Old site", "offline", true),
];

// OnTheHeader puts the picker on the view header's rail, beside a Columns ▾-style button.
function OnTheHeader({ initial, list, withoutAll }: { initial?: string; list: ProjectRow[]; withoutAll?: boolean }) {
  const [value, setValue] = useState(initial);
  const columns = usePopover();
  return (
    <div className="view-head" style={{ minHeight: 240, alignItems: "flex-start" }}>
      <ProjectPicker projects={list} value={value} withoutAll={withoutAll} onChange={setValue} />
      <div className="popover-wrap" {...columns.wrapProps}>
        <Button ref={columns.button} aria-expanded={columns.open} onClick={() => columns.setOpen((o) => !o)}>
          Columns
        </Button>
        <div className="popover-menu" hidden={!columns.open}>
          <label>
            <input type="checkbox" defaultChecked /> Ready
          </label>
        </div>
      </div>
    </div>
  );
}

export const Mixed: Story = { render: () => <OnTheHeader list={projects} initial="web" /> };
export const AllProjects: Story = { render: () => <OnTheHeader list={projects} /> };
export const SoleProject: Story = { render: () => <OnTheHeader list={[projects[0]]} initial="web" withoutAll /> };
export const UnknownKey: Story = { render: () => <OnTheHeader list={projects} initial="ghost" /> };
