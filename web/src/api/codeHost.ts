import type { Landing } from "./types";

// codeHostOf is the fake's copy of core's Code Host derivation: the Code
// Host and repository a Push remote URL names, empty when it is none Code
// Central supports. An Azure Repos repository reads organization/project/repository.
const GITHUB =
  /^(?:(?:https?|ssh|git):\/\/(?:[^@/]+@)?github\.com(?::\d+)?\/|[^@/\s]+@github\.com:)([A-Za-z0-9-]+)\/([A-Za-z0-9._-]+?)(?:\.git)?\/?$/;
const AZURE_SSH =
  /^[^@/\s]+@(?:ssh\.dev\.azure\.com|vs-ssh\.visualstudio\.com):v3\/([^/\s]+)\/([^/\s]+)\/([^/\s]+?)\/?$/;
const AZURE_HTTP =
  /^https?:\/\/(?:[^@/\s]+@)?([^/@\s]+)(\/.+?)\/_git\/([^/\s]+?)\/?$/;

export function codeHostOf(
  url: string,
): Pick<Landing, "code_host" | "repository"> {
  const remote = url.trim();
  const none = { code_host: "" as const, repository: "" };
  let m = GITHUB.exec(remote);
  if (m) return { code_host: "github", repository: `${m[1]}/${m[2]}` };
  m = AZURE_SSH.exec(remote);
  if (m)
    return { code_host: "azure_repos", repository: `${m[1]}/${m[2]}/${m[3]}` };
  m = AZURE_HTTP.exec(remote);
  if (!m) return none;
  const host = m[1].toLowerCase();
  let segs = m[2].replace(/^\/|\/$/g, "").split("/");
  if (host === "dev.azure.com" && segs.length === 2) {
    return {
      code_host: "azure_repos",
      repository: `${segs[0]}/${segs[1]}/${m[3]}`,
    };
  }
  if (host.endsWith(".visualstudio.com")) {
    if (segs.length === 2 && segs[0].toLowerCase() === "defaultcollection")
      segs = segs.slice(1);
    if (segs.length === 1) {
      return {
        code_host: "azure_repos",
        repository: `${host.slice(0, -".visualstudio.com".length)}/${segs[0]}/${m[3]}`,
      };
    }
  }
  return none;
}
