// azureProjectURL is the Azure DevOps project URL an Azure Repos git remote
// URL points at, or undefined when it isn't one (Server SSH remotes
// included). The CI section prefills the Azure DevOps project URL with it
// (code-central-368).
const SSH_SERVICES =
  /^git@ssh\.dev\.azure\.com:v3\/([^/\s]+)\/([^/\s]+)\/[^/\s]+$/;
const SSH_VISUALSTUDIO =
  /^[^@/\s]+@vs-ssh\.visualstudio\.com:v3\/([^/\s]+)\/([^/\s]+)\/[^/\s]+$/;
const HTTP_GIT =
  /^(https?:\/\/)(?:[^@/\s]+@)?([^/@\s]+)(\/.+?)\/_git\/[^/\s]+\/?$/;

export function azureProjectURL(url: string): string | undefined {
  const remote = url.trim();
  let m = SSH_SERVICES.exec(remote);
  if (m) return `https://dev.azure.com/${m[1]}/${m[2]}`;
  m = SSH_VISUALSTUDIO.exec(remote);
  if (m) return `https://${m[1]}.visualstudio.com/${m[2]}`;
  m = HTTP_GIT.exec(remote);
  return m ? `${m[1]}${m[2]}${m[3]}` : undefined;
}
