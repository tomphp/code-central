package cli_test

import (
	"context"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/codecentraltest"
)

func TestImportReadsThePayloadFromStdin(t *testing.T) {
	t.Parallel()
	srv := codecentraltest.NewServer(t)
	srv.Core.CreateProject(context.Background(), api.CreateProjectInput{Key: "acme"})
	env := map[string]string{"CODE_CENTRAL_URL": srv.URL, "CODE_CENTRAL_PROJECT": "acme"}
	stdin := `{"specs":[{"source":".scratch/login/","title":"login"}],"tickets":[]}`

	plan := decode[api.ImportResult](t, run(t, env, stdin, "import", "--dry-run"))
	if !plan.DryRun || len(plan.Specs) != 1 || plan.Specs[0].ID != "acme-1" {
		t.Fatalf("plan %+v", plan)
	}
	if res := decode[api.ImportResult](t, run(t, env, stdin, "import")); res.DryRun || res.Specs[0].ID != "acme-1" {
		t.Fatalf("result %+v", res)
	}
	// A field the payload doesn't have is rejected as strictly as the service would.
	e := wantError(t, run(t, env, `{"specs":[],"folders":[]}`, "import"), api.Invalid)
	if e.Field != "folders" {
		t.Fatalf("field %q", e.Field)
	}
}
