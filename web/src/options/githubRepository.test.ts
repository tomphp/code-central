// githubRepository reads a GitHub repository's owner/name from a git
// remote URL, to prefill the CI section (code-central-251).
import { describe, expect, it } from "vitest";
import { githubRepository } from "./githubRepository";

describe("githubRepository", () => {
  it.each([
    ["https://github.com/acme/web.git", "acme/web"],
    ["https://github.com/acme/web", "acme/web"],
    ["https://github.com/acme/web/", "acme/web"],
    ["https://someone@github.com/acme/web.git", "acme/web"],
    ["http://github.com/acme/web.site.git", "acme/web.site"],
    ["git@github.com:acme/web.git", "acme/web"],
    ["git@github.com:acme-co/web_app", "acme-co/web_app"],
    ["ssh://git@github.com/acme/web.git", "acme/web"],
    ["ssh://git@github.com:22/acme/web.git", "acme/web"],
  ])("reads %s as %s", (url, want) => {
    expect(githubRepository(url)).toBe(want);
  });

  it.each(["", "git@gitlab.com:acme/web.git", "https://github.com.evil.example/acme/web.git", "https://github.com/acme", "/srv/git/web.git"])(
    "reads nothing from %j",
    (url) => {
      expect(githubRepository(url)).toBeUndefined();
    },
  );
});
