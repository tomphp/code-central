package azdourl

import (
	"strings"
	"testing"
)

func TestParseProjectURL(t *testing.T) {
	ok := []struct{ in, want string }{
		{"https://dev.azure.com/acme/web", "https://dev.azure.com/acme/web"},
		{"  https://dev.azure.com/acme/web/  ", "https://dev.azure.com/acme/web"},
		{"https://me@dev.azure.com/acme/web", "https://dev.azure.com/acme/web"},
		{"https://user:secret@dev.azure.com/acme/web", "https://dev.azure.com/acme/web"},
		{"https://dev.azure.com/acme/web?a=1#frag", "https://dev.azure.com/acme/web"},
		{"https://dev.azure.com/acme/web/_build?definitionId=3", "https://dev.azure.com/acme/web"},
		{"https://dev.azure.com/acme/web/_git/repo/pullrequests", "https://dev.azure.com/acme/web"},
		{"https://dev.azure.com/acme/web/_settings", "https://dev.azure.com/acme/web"},
		{"http://dev.azure.com/acme/web", "http://dev.azure.com/acme/web"},
		{"https://Dev.Azure.com/acme/My%20Web", "https://dev.azure.com/acme/My%20Web"},
		{"https://dev.azure.com/acme/My Web", "https://dev.azure.com/acme/My%20Web"},
		{"https://acme.visualstudio.com/web", "https://acme.visualstudio.com/web"},
		{"https://acme.visualstudio.com/DefaultCollection/web/_git/repo", "https://acme.visualstudio.com/DefaultCollection/web"},
		{"https://ado.example.com/tfs/DefaultCollection/web", "https://ado.example.com/tfs/DefaultCollection/web"},
		{"https://ado.example.com/tfs/DefaultCollection/web/_build/", "https://ado.example.com/tfs/DefaultCollection/web"},
		{"https://ado.example.com/web-collection/web", "https://ado.example.com/web-collection/web"},
		{"http://127.0.0.1:8080/tfs/c/web", "http://127.0.0.1:8080/tfs/c/web"},
		{"https://ado.example.com:8443/a/b/c", "https://ado.example.com:8443/a/b/c"},
	}
	for _, c := range ok {
		got, err := ParseProjectURL(c.in)
		if err != nil || got != c.want {
			t.Errorf("ParseProjectURL(%q) = %q, %v; want %q", c.in, got, err, c.want)
		}
	}
	bad := []string{
		"", "   ", "dev.azure.com/acme/web", "ftp://dev.azure.com/acme/web", "https://", "not a url",
		"https://dev.azure.com", "https://dev.azure.com/acme", "https://dev.azure.com/acme/web/extra",
		"https://dev.azure.com/acme/_git/repo", "https://dev.azure.com/_build",
		"https://acme.visualstudio.com", "https://acme.visualstudio.com/Other/web", "https://acme.visualstudio.com/a/b/c",
		"https://ado.example.com", "https://ado.example.com/web", "https://ado.example.com/_git/x",
		"https://dev.azure.com/acme//web", "https://%zz/acme/web",
	}
	for _, in := range bad {
		if got, err := ParseProjectURL(in); err == nil {
			t.Errorf("ParseProjectURL(%q) = %q, want an error", in, got)
		}
	}
}

func TestParseProjectURLErrorsDoNotRepeatALogin(t *testing.T) {
	_, err := ParseProjectURL("https://me:hunter2@dev.azure.com/acme")
	if err == nil {
		t.Fatal("want an error")
	}
	if strings.Contains(err.Error(), "hunter2") {
		t.Fatalf("error repeats the login: %v", err)
	}
}
