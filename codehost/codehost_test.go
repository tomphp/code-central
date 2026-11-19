package codehost

import (
	"strings"
	"testing"
)

func TestOfWorksOutTheHostAndRepository(t *testing.T) {
	for _, c := range []struct {
		url  string
		host Host
		repo string
	}{
		{"git@github.com:acme/web.git", GitHub, "acme/web"},
		{"https://tok@github.com/acme/web", GitHub, "acme/web"},
		{"https://dev.azure.com/acme/shop/_git/web", AzureRepos, "acme/shop/web"},
		{"https://acme@dev.azure.com/acme/shop/_git/web", AzureRepos, "acme/shop/web"},
		{"git@ssh.dev.azure.com:v3/acme/shop/web", AzureRepos, "acme/shop/web"},
		{"https://acme.visualstudio.com/shop/_git/web", AzureRepos, "acme/shop/web"},
		{"https://acme.visualstudio.com/DefaultCollection/shop/_git/web", AzureRepos, "acme/shop/web"},
		{"acme@vs-ssh.visualstudio.com:v3/acme/shop/web", AzureRepos, "acme/shop/web"},
		{"https://gitlab.com/acme/web", None, ""},
		{"", None, ""},
	} {
		if host, repo := Of(c.url); host != c.host || repo != c.repo {
			t.Errorf("Of(%q) = %q, %q; want %q, %q", c.url, host, repo, c.host, c.repo)
		}
	}
}

func TestValidGitHubRepository(t *testing.T) {
	for s, want := range map[string]bool{"acme/web": true, "a-b/c.d_e": true, "acme": false, "acme/web/x": false, "": false} {
		if got := ValidGitHubRepository(s); got != want {
			t.Errorf("ValidGitHubRepository(%q) = %v", s, got)
		}
	}
}

func TestValidGitHubWorkflow(t *testing.T) {
	for s, want := range map[string]bool{"ci.yml": true, "ci.yaml": true, "a b.yml": false, "dir/ci.yml": false, "*.yml": false, "ci": false} {
		if got := ValidGitHubWorkflow(s); got != want {
			t.Errorf("ValidGitHubWorkflow(%q) = %v", s, got)
		}
	}
}

func TestCloneURLsNormaliseAcrossSchemesAndSuffixes(t *testing.T) {
	same := [][]string{
		{"git@github.com:acme/web.git", "https://github.com/acme/web", "ssh://git@github.com/Acme/Web.git/", "https://user@GitHub.com/acme/web.git"},
		{"git@ssh.dev.azure.com:v3/acme/shop/web", "https://acme@dev.azure.com/acme/shop/_git/web", "https://acme.visualstudio.com/shop/_git/web"},
		{"git@gitlab.example.com:team/web.git", "https://gitlab.example.com/team/web", "ssh://git@gitlab.example.com:2222/team/web"},
	}
	for _, group := range same {
		for _, u := range group[1:] {
			if a, b := NormalCloneURL(group[0]), NormalCloneURL(u); a != b {
				t.Errorf("%s normalises to %q and %s to %q, want the same", group[0], a, u, b)
			}
		}
	}
	for _, pair := range [][2]string{
		{"git@github.com:acme/web.git", "git@github.com:acme/api.git"},
		{"git@github.com:acme/web.git", "git@gitlab.com:acme/web.git"},
	} {
		if NormalCloneURL(pair[0]) == NormalCloneURL(pair[1]) {
			t.Errorf("%s and %s normalise the same", pair[0], pair[1])
		}
	}
	if got := NormalCloneURL(" /srv/git/web.git/ "); !strings.HasSuffix(got, "/srv/git/web") {
		t.Errorf("a local path normalises to %q", got)
	}
}
