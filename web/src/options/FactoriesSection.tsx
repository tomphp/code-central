import type { ProjectRow } from "../api/types";
import { FactoryList, NO_HEARTBEATS } from "../factory/FactoryStatus";

// FactoriesSection lists the Project's Factories, as the dot's tooltip does.
export function FactoriesSection({
  project,
  now,
}: {
  project: ProjectRow;
  now: Date;
}) {
  const status = project.factory_status;
  return (
    <fieldset className="project-options-section">
      <legend>Factories</legend>
      {status.factories.length === 0 ? (
        <p className="factory-empty">
          {NO_HEARTBEATS} See{" "}
          <a href="docs/running-the-factory.md">running a Factory</a> and{" "}
          <a href="docs/writing-a-factory.md">writing a Factory</a>.
        </p>
      ) : (
        <FactoryList status={status} now={now} />
      )}
    </fieldset>
  );
}
