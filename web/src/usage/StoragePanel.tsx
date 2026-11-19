// StoragePanel is Usage's Storage tab (Spec code-central-1032): where the
// Store is and its size, each Project's count of Specs, Tickets and Usage
// days, this service's memory, and the services registered on the Store.
// The counts are the scoped Project's alone, or every Project's; the
// memory and services are always whole. A Postgres Store shows its host,
// port and database, which is all the read carries of its URL.
import type { StorageView } from "../api/types";
import { ago, bytes, count } from "../format";

// KINDS are the Store Kinds' names; an unknown Kind shows as it came.
const KINDS: Record<string, string> = { files: "Files", sqlite: "SQLite", postgres: "Postgres", memory: "In memory" };

export interface StoragePanelProps {
  storage: StorageView;
  // project is the scope's Project; left out, every Project's counts show.
  project?: string;
  now: Date;
}

export function StoragePanel({ storage, project, now }: StoragePanelProps) {
  const projects = project ? storage.projects.filter((p) => p.key === project) : storage.projects;
  const location = storage.host ? `${storage.host}:${storage.port}/${storage.database}` : storage.path;
  return (
    <>
      <h3 className="usage-subhead">Store</h3>
      <table aria-label="Store" className="data-table data-table-mono">
        <tbody>
          <tr>
            <th scope="row">Kind</th>
            <td>{KINDS[storage.kind] ?? storage.kind}</td>
          </tr>
          {location && (
            <tr>
              <th scope="row">Location</th>
              <td>{location}</td>
            </tr>
          )}
          <tr>
            <th scope="row">Size</th>
            <ByteCell n={storage.size} />
          </tr>
        </tbody>
      </table>
      <h3 className="usage-subhead">Projects' records</h3>
      <table aria-label="Projects' records" className="data-table data-table-mono">
        <thead>
          <tr>
            <th scope="col">Project</th>
            <th scope="col" className="num">Specs</th>
            <th scope="col" className="num">Tickets</th>
            <th scope="col" className="num">Usage days</th>
          </tr>
        </thead>
        <tbody>
          {projects.map((p) => (
            <tr key={p.key}>
              <th scope="row">{p.name}</th>
              <td className="num">{count(p.specs)}</td>
              <td className="num">{count(p.tickets)}</td>
              <td className="num">{count(p.usage_days)}</td>
            </tr>
          ))}
        </tbody>
      </table>
      <h3 className="usage-subhead">Memory</h3>
      <table aria-label="Memory" className="data-table data-table-mono">
        <tbody>
          {!!storage.memory.resident && (
            <tr>
              <th scope="row">Resident</th>
              <ByteCell n={storage.memory.resident} />
            </tr>
          )}
          <tr>
            <th scope="row">Go heap</th>
            <ByteCell n={storage.memory.heap} />
          </tr>
        </tbody>
      </table>
      <h3 className="usage-subhead">Services</h3>
      <table aria-label="Services" className="data-table data-table-mono">
        <thead>
          <tr>
            <th scope="col">Host</th>
            <th scope="col">Version</th>
            <th scope="col">Renewed</th>
          </tr>
        </thead>
        <tbody>
          {storage.services.map((s) => (
            <tr key={`${s.host} ${s.version} ${s.renewed}`}>
              <th scope="row">{s.host}</th>
              <td>{s.version}</td>
              <td title={s.renewed}>{ago(now, s.renewed)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </>
  );
}

// ByteCell is a size's cell, in readable units, with the exact figure on hover.
function ByteCell({ n }: { n: number }) {
  return (
    <td className="num" title={`${count(n)} bytes`}>
      {bytes(n)}
    </td>
  );
}
