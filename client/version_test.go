package client_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tomoram/code-central/client"
)

// A newer service may add response fields; an older client keeps working,
// and sees the service's version header.
func TestTheClientToleratesUnknownResponseFields(t *testing.T) {
	hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Code-Central-Version", "v9.0.0")
		w.Write([]byte(`{"key":"acme","display_name":"Acme","added_in_v9":{"x":1}}`))
	}))
	defer hs.Close()
	c := client.New(hs.URL)
	var seen string
	c.OnVersion = func(v string) { seen = v }
	p, err := c.GetProject(ctx, "acme")
	if err != nil || p.Key != "acme" || p.DisplayName != "Acme" {
		t.Fatalf("get_project: %+v, %v", p, err)
	}
	if seen != "v9.0.0" {
		t.Fatalf("OnVersion saw %q", seen)
	}
}
