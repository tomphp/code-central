package frontdoor_test

import (
	"testing"

	"github.com/tomoram/code-central/core"
)

// A Factory's Heartbeat (Spec code-central-305): report_heartbeat, over
// HTTP only, and the factory_status the Project reads carry.
func init() {
	cases["report_heartbeat"] = opCase{
		ok: call{http: reportHeartbeat(`{"factory":"code-central-factory","host":"box","state":"online","note":"1 of 2 turns running","interval_seconds":10}`)},
		check: func(t *testing.T, out map[string]any) {
			t.Helper()
			acknowledgement("acme")(t, out)
		},
		full: func(t *testing.T, out map[string]any) {
			t.Helper()
			fs, _ := out["factory_status"].(map[string]any)
			field("status", "online")(t, fs)
			factories, _ := fs["factories"].([]any)
			if len(factories) != 1 {
				t.Fatalf("factories = %v, want one", fs["factories"])
			}
			f, _ := factories[0].(map[string]any)
			field("name", "code-central-factory")(t, f)
			field("host", "box")(t, f)
			field("status", "online")(t, f)
			field("note", "1 of 2 turns running")(t, f)
			if f["interval_seconds"] != float64(10) {
				t.Fatalf("interval_seconds = %v", f["interval_seconds"])
			}
		},
		bad:  call{http: reportHeartbeat(`{"factory":"f","host":"h","state":"asleep","interval_seconds":10}`)},
		code: core.Invalid,
	}
}

func reportHeartbeat(body string) httpCall {
	return httpCall{"POST", "/api/p/acme/report_heartbeat", body}
}

func TestAHeartbeatShowsInTheProjectReads(t *testing.T) {
	srv := newServer(t)
	for _, c := range []httpCall{{"GET", "/api/projects/acme", ""}} {
		out := httpOK(t, srv, c)
		fs, _ := out["factory_status"].(map[string]any)
		field("status", "offline")(t, fs)
		if f, ok := fs["factories"].([]any); !ok || len(f) != 0 {
			t.Fatalf("factories = %v, want []", fs["factories"])
		}
	}
	httpOK(t, srv, reportHeartbeat(`{"factory":"b","host":"h","state":"draining","interval_seconds":5}`))
	httpOK(t, srv, reportHeartbeat(`{"factory":"a","host":"h","state":"online","interval_seconds":5}`))

	get := httpOK(t, srv, httpCall{"GET", "/api/projects/acme", ""})
	check := func(where string, p map[string]any) {
		t.Helper()
		fs, _ := p["factory_status"].(map[string]any)
		field("status", "online")(t, fs)
		fl, _ := fs["factories"].([]any)
		if len(fl) != 2 || fl[0].(map[string]any)["name"] != "a" || fl[1].(map[string]any)["status"] != "draining" {
			t.Fatalf("%s factories = %v", where, fs["factories"])
		}
	}
	check("get_project", get)

	for _, p := range httpOK(t, srv, httpCall{"GET", "/api/projects", ""})["rows"].([]any) {
		if m := p.(map[string]any); m["key"] == "acme" {
			check("list_projects", m)
			return
		}
	}
	t.Fatal("acme not in list_projects")
}

func TestAnInvalidHeartbeatNamesTheField(t *testing.T) {
	srv := newServer(t)
	for field, body := range map[string]string{
		"factory":          `{"factory":" ","host":"h","state":"online","interval_seconds":5}`,
		"host":             `{"factory":"f","state":"online","interval_seconds":5}`,
		"state":            `{"factory":"f","host":"h","interval_seconds":5}`,
		"interval_seconds": `{"factory":"f","host":"h","state":"online","interval_seconds":86401}`,
	} {
		status, b := httpDo(t, srv, reportHeartbeat(body))
		out := object(t, b)
		if status == 200 || out["field"] != field {
			t.Errorf("%s: status %d, %s", field, status, b)
		}
	}
}
