// githubRepository is the owner/name of the GitHub repository a git remote
// URL points at, HTTPS or SSH, or undefined when it isn't GitHub's. The CI
// section prefills the GitHub repository with it (code-central-251).
const GITHUB_URL =
  /^(?:(?:https?|ssh|git):\/\/(?:[^@/]+@)?github\.com(?::\d+)?\/|[^@/\s]+@github\.com:)([A-Za-z0-9-]+)\/([A-Za-z0-9._-]+?)(?:\.git)?\/?$/;

export function githubRepository(url: string): string | undefined {
  const m = GITHUB_URL.exec(url.trim());
  return m ? `${m[1]}/${m[2]}` : undefined;
}
