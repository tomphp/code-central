// The Projects select of a full-width view: All Projects, every active
// Project, Archived ones marked, and the chosen Project when it isn't known.
import type { ProjectRow } from "../api/types";

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
      <select value={value ?? ""} onChange={(e) => onChange(e.target.value || undefined)}>
        <option value="">All Projects</option>
        {projects.map((p) => (
          <option key={p.key} value={p.key}>
            {p.archived ? `${p.display_name} (Archived)` : p.display_name}
          </option>
        ))}
        {value && !projects.some((p) => p.key === value) && <option value={value}>{value}</option>}
      </select>
    </label>
  );
}
