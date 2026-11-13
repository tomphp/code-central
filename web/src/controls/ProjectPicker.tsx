// The Projects select of a full-width view: All Projects, every active
// Project, Archived ones marked, and the chosen Project when it isn't known.
import type { ProjectRow } from "../api/types";
import { Select } from "./Select";

export interface ProjectPickerProps {
  projects: ProjectRow[];
  // value is the chosen Project's key, undefined for All Projects.
  value?: string;
  onChange(key: string | undefined): void;
}

export function ProjectPicker({ projects, value, onChange }: ProjectPickerProps) {
  return (
    <label>
      <span className="view-picker-label">Projects</span>
      <Select
        value={value ?? ""}
        placeholder="All Projects"
        onChange={(v) => onChange(v || undefined)}
        options={[
          ...projects.map((p) => ({ value: p.key, label: p.archived ? `${p.display_name} (Archived)` : p.display_name })),
          ...(value && !projects.some((p) => p.key === value) ? [{ value, label: value }] : []),
        ]}
      />
    </label>
  );
}
