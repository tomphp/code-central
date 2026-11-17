package codehost

import "testing"

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
