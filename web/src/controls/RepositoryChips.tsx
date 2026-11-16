// Repository chips (Spec code-central-848): the Repository Names a Ticket
// or a Spec changes, its Changed Repositories, as small name chips. Reads
// carry them only while the Project has more than one Repository, so in a
// single-repository Project there are none to show.
export function RepositoryChips({ names }: { names?: string[] }) {
  if (!names || names.length === 0) return null;
  return (
    <span className="repo-chips" role="list" aria-label="Changed Repositories">
      {names.map((n) => (
        <span key={n} role="listitem" className="label repo-chip">
          {n}
        </span>
      ))}
    </span>
  );
}
