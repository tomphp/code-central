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

func TestParseQueryURL(t *testing.T) {
	const id = "6b4a0c1e-9f1d-4a51-8f43-2a7d3c1b9e10"
	ok := []struct{ in, want string }{
		{"https://dev.azure.com/contoso/web/_queries/query/" + id, "https://dev.azure.com/contoso/web/_queries/query/" + id},
		{"  https://dev.azure.com/contoso/web/_queries/query/" + id + "/  ", "https://dev.azure.com/contoso/web/_queries/query/" + id},
		{"https://dev.azure.com/contoso/web/_queries/query-edit/" + id + "/?a=1#frag", "https://dev.azure.com/contoso/web/_queries/query/" + id},
		{"https://me:secret@Dev.Azure.com/contoso/My Web/_queries/query/" + strings.ToUpper(id), "https://dev.azure.com/contoso/My%20Web/_queries/query/" + strings.ToUpper(id)},
		{"https://contoso.visualstudio.com/web/_queries/query/" + id, "https://contoso.visualstudio.com/web/_queries/query/" + id},
		{"https://ado.example.com/tfs/DefaultCollection/web/_queries/query/" + id, "https://ado.example.com/tfs/DefaultCollection/web/_queries/query/" + id},
	}
	for _, c := range ok {
		got, err := ParseQueryURL(c.in)
		if err != nil || got != c.want {
			t.Errorf("ParseQueryURL(%q) = %q, %v; want %q", c.in, got, err, c.want)
		}
	}
	bad := []string{
		"", "https://dev.azure.com/contoso/web", "https://dev.azure.com/contoso/web/_queries",
		"https://dev.azure.com/contoso/web/_queries/query", "https://dev.azure.com/contoso/web/_queries/query/not-a-guid",
		"https://dev.azure.com/contoso/web/_queries/query/" + id + "/extra", "https://dev.azure.com/contoso/web/_build/query/" + id,
		"https://dev.azure.com/contoso/_queries/query/" + id, "dev.azure.com/contoso/web/_queries/query/" + id,
	}
	for _, in := range bad {
		if got, err := ParseQueryURL(in); err == nil {
			t.Errorf("ParseQueryURL(%q) = %q, want an error", in, got)
		}
	}
	if _, err := ParseQueryURL("https://me:hunter2@dev.azure.com/contoso/web"); err == nil || strings.Contains(err.Error(), "hunter2") {
		t.Errorf("error = %v, want one that doesn't repeat the login", err)
	}
}

func TestNames(t *testing.T) {
	cases := []struct{ in, org, project string }{
		{"https://dev.azure.com/contoso/web", "contoso", "web"},
		{"https://dev.azure.com/Contoso/My%20Web", "Contoso", "My Web"},
		{"https://contoso.visualstudio.com/web", "contoso", "web"},
		{"https://contoso.visualstudio.com/DefaultCollection/web", "contoso", "web"},
		{"https://ado.example.com/tfs/DefaultCollection/web", "DefaultCollection", "web"},
		{"https://dev.azure.com/contoso/web/_queries/query/6b4a0c1e-9f1d-4a51-8f43-2a7d3c1b9e10", "contoso", "web"},
	}
	for _, c := range cases {
		if org, project := Names(c.in); org != c.org || project != c.project {
			t.Errorf("Names(%q) = %q, %q; want %q, %q", c.in, org, project, c.org, c.project)
		}
	}
	if org, project := Names("not a url"); org != "" || project != "" {
		t.Errorf("Names of no URL = %q, %q; want none", org, project)
	}
}
