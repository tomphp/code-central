package azdo_test

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/azdo"
	"github.com/tomoram/code-central/internal/fakeazdo"
	"github.com/tomoram/code-central/watchers"
)

const pat = "s3cr3t-pat-value"

func client(t *testing.T, fake *fakeazdo.Server) *azdo.Client {
	t.Helper()
	c, err := azdo.New([]string{"OTHER=x", "ADO_PAT=" + pat}, "ADO_PAT", fake.ProjectURL(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestAMissingOrEmptyPATIsItsOwnError(t *testing.T) {
	for _, env := range [][]string{nil, {"ADO_PAT="}, {"ADO_PAT=  "}, {"OTHER=x"}} {
		_, err := azdo.New(env, "ADO_PAT", "https://dev.azure.com/acme/web", nil)
		if !errors.Is(err, azdo.ErrNoPAT) || !strings.Contains(err.Error(), "ADO_PAT") {
			t.Errorf("env %v: err = %v, want ErrNoPAT naming ADO_PAT", env, err)
		}
	}
}

func TestANewClientRefusesAProjectURLThatDoesNotParse(t *testing.T) {
	if _, err := azdo.New([]string{"ADO_PAT=" + pat}, "ADO_PAT", "https://dev.azure.com/acme", nil); err == nil {
		t.Fatal("want an error")
	}
}

func TestTheLastEnvironmentEntryWins(t *testing.T) {
	fake := fakeazdo.New(t)
	c, err := azdo.New([]string{"P=old", "P=" + pat}, "P", fake.ProjectURL(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Builds(context.Background(), azdo.BuildQuery{}); err != nil {
		t.Fatal(err)
	}
	if got := fake.Requests()[0].Authorization; got != basic(pat) {
		t.Fatalf("Authorization = %q", got)
	}
}

func basic(p string) string { return "Basic " + base64.StdEncoding.EncodeToString([]byte(":"+p)) }

func TestCallsSendThePATAsBasicAuthAndPinTheAPIVersion(t *testing.T) {
	fake := fakeazdo.New(t)
	if _, err := client(t, fake).Builds(context.Background(), azdo.BuildQuery{}); err != nil {
		t.Fatal(err)
	}
	r := fake.Requests()[0]
	if r.Authorization != basic(pat) {
		t.Errorf("Authorization = %q, want %q", r.Authorization, basic(pat))
	}
	if r.Path != fakeazdo.ProjectPath+"/_apis/build/builds" {
		t.Errorf("path = %q", r.Path)
	}
	if r.Query["api-version"][0] != azdo.APIVersion {
		t.Errorf("api-version = %v", r.Query["api-version"])
	}
}

func TestProjectReadsTheProjectResourceUnderTheCollection(t *testing.T) {
	fake := fakeazdo.New(t, fakeazdo.JSON("GET", "/tfs/DefaultCollection/_apis/projects/web", 200, map[string]any{"id": "1", "name": "web"}))
	p, err := client(t, fake).Project(context.Background())
	if err != nil || p.Name != "web" {
		t.Fatalf("Project = %+v, %v", p, err)
	}
}

func TestBuildsPassTheirQueryOptionsAndDecode(t *testing.T) {
	fake := fakeazdo.New(t, fakeazdo.JSON("GET", "/", 200, map[string]any{"value": []map[string]any{{
		"id": 7, "buildNumber": "20261111.1", "status": "completed", "result": "partiallySucceeded",
		"sourceBranch": "refs/heads/main", "sourceVersion": "abc",
		"finishTime":      "2026-11-11T10:00:00Z",
		"lastChangedDate": "2026-11-11T10:00:01Z",
		"definition":      map[string]any{"name": "CI", "path": `\team`},
		"_links":          map[string]any{"web": map[string]any{"href": "https://x/build/7"}},
	}}}))
	min := time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)
	bs, err := client(t, fake).Builds(context.Background(), azdo.BuildQuery{
		Top: 200, Order: "finishTimeDescending", Branch: "refs/heads/main", Status: "completed", MinTime: min,
	})
	if err != nil || len(bs) != 1 {
		t.Fatalf("Builds = %+v, %v", bs, err)
	}
	b := bs[0]
	if b.ID != 7 || b.Result != "partiallySucceeded" || b.Definition.Name != "CI" || b.Links.Web.Href != "https://x/build/7" || b.FinishTime.IsZero() || !b.StartTime.IsZero() {
		t.Errorf("build = %+v", b)
	}
	q := fake.Requests()[0].Query
	for k, want := range map[string]string{
		"$top": "200", "queryOrder": "finishTimeDescending", "branchName": "refs/heads/main",
		"statusFilter": "completed", "minTime": "2026-11-11T09:00:00Z",
	} {
		if q[k][0] != want {
			t.Errorf("query %s = %v, want %s", k, q[k], want)
		}
	}
}

func TestTimelineReadsRecords(t *testing.T) {
	fake := fakeazdo.New(t, fakeazdo.JSON("GET", fakeazdo.ProjectPath+"/_apis/build/builds/7/timeline", 200,
		map[string]any{"records": []map[string]any{{"id": "j1", "type": "Job", "name": "build", "state": "completed", "result": "failed", "attempt": 2}}}))
	rs, err := client(t, fake).Timeline(context.Background(), 7)
	if err != nil || len(rs) != 1 || rs[0].Type != "Job" || rs[0].Attempt != 2 {
		t.Fatalf("Timeline = %+v, %v", rs, err)
	}
}

func TestRetryPatchesTheBuildWithRetry(t *testing.T) {
	fake := fakeazdo.New(t)
	if err := client(t, fake).Retry(context.Background(), 7); err != nil {
		t.Fatal(err)
	}
	r := fake.Requests()[0]
	if r.Method != "PATCH" || r.Path != fakeazdo.ProjectPath+"/_apis/build/builds/7" || r.Query["retry"][0] != "true" || !strings.Contains(r.Body, `"retry":true`) {
		t.Fatalf("request = %+v", r)
	}
}

func TestRefusalsBecomeErrors(t *testing.T) {
	cases := []struct {
		status int
		want   error
	}{{401, azdo.ErrPATRejected}, {203, azdo.ErrPATRejected}, {302, azdo.ErrPATRejected}, {403, azdo.ErrMissingScope}, {404, azdo.ErrNotFound}}
	for _, c := range cases {
		fake := fakeazdo.New(t, fakeazdo.Answer{Status: c.status, Path: "/", Body: "nope"})
		_, err := client(t, fake).Builds(context.Background(), azdo.BuildQuery{})
		if !errors.Is(err, c.want) {
			t.Errorf("status %d: err = %v, want %v", c.status, err, c.want)
		}
	}
	fake := fakeazdo.New(t, fakeazdo.Answer{Status: 500, Path: "/", Body: "boom\nmore"})
	_, err := client(t, fake).Builds(context.Background(), azdo.BuildQuery{})
	if err == nil || !strings.Contains(err.Error(), "500") || !strings.Contains(err.Error(), "boom") {
		t.Errorf("500: err = %v", err)
	}
}

func TestThrottlingIsTheSharedRateLimitError(t *testing.T) {
	for _, status := range []int{429, 503} {
		a := fakeazdo.Answer{Status: status, Path: "/"}.WithHeader("Retry-After", "30").WithHeader("X-RateLimit-Reset", "1900000000")
		_, err := client(t, fakeazdo.New(t, a)).Builds(context.Background(), azdo.BuildQuery{})
		var rl *watchers.RateLimitError
		if !errors.As(err, &rl) {
			t.Fatalf("status %d: err = %v, want a rate limit", status, err)
		}
		if rl.System != "Azure DevOps" || rl.RetryAfter != 30*time.Second || rl.Reset.Unix() != 1900000000 {
			t.Errorf("status %d: limit = %+v", status, rl)
		}
	}
}

func TestA503WithoutRetryAfterIsNotARateLimit(t *testing.T) {
	_, err := client(t, fakeazdo.New(t, fakeazdo.Answer{Status: 503, Path: "/"})).Builds(context.Background(), azdo.BuildQuery{})
	var rl *watchers.RateLimitError
	if err == nil || errors.As(err, &rl) {
		t.Fatalf("err = %v", err)
	}
}

func TestASuccessCarryingOnlyXRateLimitDelayIsNotARefusal(t *testing.T) {
	a := fakeazdo.Answer{Path: "/", Body: `{"value":[]}`}.WithHeader("X-RateLimit-Delay", "5")
	if _, err := client(t, fakeazdo.New(t, a)).Builds(context.Background(), azdo.BuildQuery{}); err != nil {
		t.Fatal(err)
	}
}

func TestThePATNeverAppearsInErrors(t *testing.T) {
	check := func(err error) {
		t.Helper()
		if err != nil && (strings.Contains(err.Error(), pat) || strings.Contains(err.Error(), base64.StdEncoding.EncodeToString([]byte(":"+pat)))) {
			t.Errorf("error holds the PAT: %v", err)
		}
	}
	for _, status := range []int{401, 403, 404, 429, 500} {
		// An answer that echoes the PAT back must not reach the error either for statuses with no body line.
		fake := fakeazdo.New(t, fakeazdo.Answer{Status: status, Path: "/", Body: "bad"})
		c := client(t, fake)
		_, err := c.Builds(context.Background(), azdo.BuildQuery{})
		check(err)
		_, err = c.Project(context.Background())
		check(err)
		_, err = c.Timeline(context.Background(), 1)
		check(err)
		check(c.Retry(context.Background(), 1))
	}
	fake := fakeazdo.New(t)
	fake.Close()
	_, err := client(t, fake).Builds(context.Background(), azdo.BuildQuery{})
	if err == nil {
		t.Fatal("want a connection error")
	}
	check(err)
	_, err = azdo.New([]string{"P=" + pat}, "OTHER", "https://dev.azure.com/acme/web", nil)
	check(err)
	_, err = azdo.New([]string{"P=" + pat}, "P", "https://u:"+pat+"@dev.azure.com/acme", nil)
	check(err)
}

func TestARedirectIsNotFollowed(t *testing.T) {
	login := fakeazdo.New(t)
	fake := fakeazdo.New(t, fakeazdo.Answer{Status: http.StatusFound, Path: "/"}.WithHeader("Location", login.URL+"/signin"))
	_, err := client(t, fake).Builds(context.Background(), azdo.BuildQuery{})
	if !errors.Is(err, azdo.ErrPATRejected) || len(login.Requests()) != 0 {
		t.Fatalf("err = %v, login requests = %d", err, len(login.Requests()))
	}
}
