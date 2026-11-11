// azureProjectURL reads an Azure DevOps project URL from an Azure Repos
// Push remote, to prefill the CI section (code-central-368).
import { describe, expect, it } from "vitest";
import { azureProjectURL } from "./azureProjectURL";

describe("azureProjectURL", () => {
  it.each([
    ["https://dev.azure.com/acme/web/_git/app", "https://dev.azure.com/acme/web"],
    ["https://someone@dev.azure.com/acme/web/_git/app", "https://dev.azure.com/acme/web"],
    ["https://dev.azure.com/acme/My%20Web/_git/app", "https://dev.azure.com/acme/My%20Web"],
    ["git@ssh.dev.azure.com:v3/acme/web/app", "https://dev.azure.com/acme/web"],
    ["git@ssh.dev.azure.com:v3/acme/My%20Web/app", "https://dev.azure.com/acme/My%20Web"],
    ["https://acme.visualstudio.com/web/_git/app", "https://acme.visualstudio.com/web"],
    ["https://acme.visualstudio.com/DefaultCollection/web/_git/app", "https://acme.visualstudio.com/DefaultCollection/web"],
    ["https://acme.visualstudio.com/web/_git/app/", "https://acme.visualstudio.com/web"],
    ["acme@vs-ssh.visualstudio.com:v3/acme/web/app", "https://acme.visualstudio.com/web"],
    ["https://tfs.example.com/tfs/DefaultCollection/web/_git/app", "https://tfs.example.com/tfs/DefaultCollection/web"],
    ["http://tfs.example.com:8080/tfs/DefaultCollection/web/_git/app", "http://tfs.example.com:8080/tfs/DefaultCollection/web"],
  ])("reads %s as %s", (url, want) => {
    expect(azureProjectURL(url)).toBe(want);
  });

  it.each([
    "",
    "git@github.com:acme/web.git",
    "https://github.com/acme/web.git",
    "ssh://git@tfs.example.com:22/tfs/DefaultCollection/web/_git/app",
    "git@tfs.example.com:acme/web.git",
    "https://dev.azure.com/acme/web",
    "/srv/git/web.git",
  ])("reads nothing from %j", (url) => {
    expect(azureProjectURL(url)).toBeUndefined();
  });
});
