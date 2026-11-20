//go:build js && wasm

// Command corewasm is the WASM Core (Spec code-central-580): the world
// package bound to syscall/js, for the UI's tests and stories to run on.
// `make corewasm` builds it into web/src/api/core/core.wasm. It holds no
// logic: each function turns JavaScript values into a world's call and
// its answer back. It sets globalThis.codeCentralCore to:
//
//	newWorld(startISO) -> id
//	fetch(id, method, url, headers, body?, signal?) -> Promise<Response>
//	now(id) -> ISO time
//	advance(id, ms)
//	setCICheck(id, resultJSON), setIssuesCheck(id, resultJSON),
//	setAzureBoardsCheck(id, resultJSON), setPullRequestCheck(id, resultJSON)
//	reportWatcherCrash(id, kind, key, restarts, lastLine, restartAtISO)
//	holdWatcherLease(id, key, host)
//	postSessionEventsOn(id, host, sessionID, eventsJSON)
//	dropStreams(id)
//	transcriptStreams(id) -> count
//	settled(id, events, transcripts, chats) -> busyJSON
//	drop(id)
//
// fetch's headers are [name, value] pairs and its body a string; its
// Response's body streams as the handler writes it, and cancelling that
// body or aborting signal ends the request. A check's result is
// core.WatcherCheckResult as JSON, "{}" to pass, and eventsJSON is
// []core.SessionEvent as JSON. settled takes how many
// feeds of each stream kind the client has subscribed and answers
// world.Busy as JSON, "{}" once the world is settled. Using an id after
// drop is a test's bug, and stops the program.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"syscall/js"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/world"
)

var (
	worlds = map[int]*world.World{}
	nextID = 1
)

func main() {
	api := js.Global().Get("Object").New()
	set := func(name string, fn func(a []js.Value) any) {
		api.Set(name, js.FuncOf(func(_ js.Value, a []js.Value) any { return fn(a) }))
	}
	set("newWorld", func(a []js.Value) any {
		id := nextID
		nextID++
		worlds[id] = world.New(parseTime(a[0]))
		return id
	})
	set("fetch", fetch)
	set("now", func(a []js.Value) any { return of(a[0]).Now().UTC().Format(time.RFC3339Nano) })
	set("advance", func(a []js.Value) any {
		of(a[0]).Advance(time.Duration(a[1].Float() * float64(time.Millisecond)))
		return nil
	})
	set("setCICheck", func(a []js.Value) any { of(a[0]).SetCICheck(checkResult(a[1])); return nil })
	set("setIssuesCheck", func(a []js.Value) any { of(a[0]).SetIssuesCheck(checkResult(a[1])); return nil })
	set("setAzureBoardsCheck", func(a []js.Value) any { of(a[0]).SetAzureBoardsCheck(checkResult(a[1])); return nil })
	set("setPullRequestCheck", func(a []js.Value) any { of(a[0]).SetPullRequestCheck(checkResult(a[1])); return nil })
	set("reportWatcherCrash", func(a []js.Value) any {
		of(a[0]).ReportWatcherCrash(core.WatcherKind(a[1].String()), a[2].String(), core.WatcherCrash{
			Restarts: a[3].Int(), LastLine: a[4].String(), RestartAt: parseTime(a[5]),
		})
		return nil
	})
	set("holdWatcherLease", func(a []js.Value) any { of(a[0]).HoldWatcherLease(a[1].String(), a[2].String()); return nil })
	set("postSessionEventsOn", func(a []js.Value) any {
		var events []core.SessionEvent
		if err := json.Unmarshal([]byte(a[3].String()), &events); err != nil {
			panic("corewasm: Session Events: " + err.Error())
		}
		of(a[0]).PostSessionEventsOn(a[1].String(), a[2].String(), events)
		return nil
	})
	set("dropStreams", func(a []js.Value) any { of(a[0]).DropStreams(); return nil })
	set("transcriptStreams", func(a []js.Value) any { return of(a[0]).TranscriptStreams() })
	set("settled", func(a []js.Value) any {
		busy, _ := json.Marshal(of(a[0]).Settled(world.Subscribed{
			Events: a[1].Int(), Transcripts: a[2].Int(), Chats: a[3].Int(),
		}))
		return string(busy)
	})
	set("drop", func(a []js.Value) any {
		of(a[0]).Drop()
		delete(worlds, a[0].Int())
		return nil
	})
	js.Global().Set("codeCentralCore", api)
	select {}
}

// of is the world id names.
func of(id js.Value) *world.World {
	w, ok := worlds[id.Int()]
	if !ok {
		panic("corewasm: no world " + id.String())
	}
	return w
}

func parseTime(v js.Value) time.Time {
	t, err := time.Parse(time.RFC3339Nano, v.String())
	if err != nil {
		panic("corewasm: " + err.Error())
	}
	return t
}

func checkResult(v js.Value) core.WatcherCheckResult {
	var res core.WatcherCheckResult
	if err := json.Unmarshal([]byte(v.String()), &res); err != nil {
		panic("corewasm: a check's result: " + err.Error())
	}
	return res
}

// fetch serves one request through the world and resolves to its
// Response once the handler has written its status.
func fetch(a []js.Value) any {
	w := of(a[0])
	method, url, headers, body := a[1].String(), a[2].String(), a[3], arg(a, 4)
	f := newFetching(arg(a, 5))
	return promise(func() (js.Value, error) {
		var r io.Reader
		if body.Type() == js.TypeString {
			r = strings.NewReader(body.String())
		}
		req, err := http.NewRequestWithContext(f.ctx, method, url, r)
		if err != nil {
			f.end()
			return js.Value{}, err
		}
		for i := range headers.Length() {
			req.Header.Add(headers.Index(i).Index(0).String(), headers.Index(i).Index(1).String())
		}
		res, err := w.RoundTrip(req)
		if err != nil {
			f.end()
			return js.Value{}, f.reason(err)
		}
		return f.response(res), nil
	})
}

// arg is a's i'th value, undefined past its end.
func arg(a []js.Value, i int) js.Value {
	if i < len(a) {
		return a[i]
	}
	return js.Undefined()
}

// fetching is one fetch's request, from the call until its body ends.
type fetching struct {
	ctx    context.Context
	cancel context.CancelFunc
	// signal is the fetch's AbortSignal, or undefined.
	signal  js.Value
	onAbort js.Func
	// funcs are the JavaScript functions it made, freed when it ends.
	funcs []js.Func
	once  sync.Once
	// cancelled is set once its body's reader cancels it, after which
	// the stream refuses a chunk or a close with an exception.
	cancelled bool
}

func newFetching(signal js.Value) *fetching {
	f := &fetching{signal: signal}
	f.ctx, f.cancel = context.WithCancel(context.Background())
	if signal.Truthy() {
		if signal.Get("aborted").Bool() {
			f.cancel()
		}
		f.onAbort = js.FuncOf(func(js.Value, []js.Value) any { f.cancel(); return nil })
		f.funcs = append(f.funcs, f.onAbort)
		signal.Call("addEventListener", "abort", f.onAbort)
	}
	return f
}

// end ends the request and frees its functions. It runs on a goroutine,
// never inside one of them.
func (f *fetching) end() {
	f.once.Do(func() {
		f.cancel()
		if f.signal.Truthy() {
			f.signal.Call("removeEventListener", "abort", f.onAbort)
		}
		for _, fn := range f.funcs {
			fn.Release()
		}
	})
}

// reason is what an ended request rejects with: its signal's reason once
// aborted, as a fetch's, else err.
func (f *fetching) reason(err error) error {
	if f.signal.Truthy() && f.signal.Get("aborted").Bool() {
		return rejection{f.signal.Get("reason")}
	}
	return err
}

// response is res as a JavaScript Response whose body reads res's body
// as it streams.
func (f *fetching) response(res *http.Response) js.Value {
	headers := js.Global().Get("Headers").New()
	for name, values := range res.Header {
		for _, v := range values {
			headers.Call("append", name, v)
		}
	}
	init := map[string]any{"status": res.StatusCode, "headers": headers}
	switch res.StatusCode {
	case http.StatusNoContent, http.StatusResetContent, http.StatusNotModified:
		// A Response with one of these statuses can't have a body.
		res.Body.Close()
		f.end()
		return js.Global().Get("Response").New(js.Null(), init)
	}
	pull := js.FuncOf(func(_ js.Value, a []js.Value) any {
		ctrl := a[0]
		return promise(func() (js.Value, error) {
			buf := make([]byte, 32<<10)
			n, err := res.Body.Read(buf)
			// A cancel can come between the read and here, and an
			// exception from the stream would stop every world.
			if f.cancelled {
				f.end()
				return js.Undefined(), nil
			}
			if n > 0 {
				chunk := js.Global().Get("Uint8Array").New(n)
				js.CopyBytesToJS(chunk, buf[:n])
				ctrl.Call("enqueue", chunk)
				return js.Undefined(), nil
			}
			f.end()
			// An aborted fetch's body fails however its handler ended.
			if err := f.reason(err); !errors.Is(err, io.EOF) {
				return js.Value{}, err
			}
			ctrl.Call("close")
			return js.Undefined(), nil
		})
	})
	cancel := js.FuncOf(func(js.Value, []js.Value) any {
		f.cancelled = true
		res.Body.Close()
		go f.end()
		return nil
	})
	f.funcs = append(f.funcs, pull, cancel)
	stream := js.Global().Get("ReadableStream").New(map[string]any{"pull": pull, "cancel": cancel})
	return js.Global().Get("Response").New(stream, init)
}

// rejection is an error that rejects a promise with a JavaScript value
// of its own, as an aborted fetch rejects with its signal's reason.
type rejection struct{ reason js.Value }

func (r rejection) Error() string { return "aborted" }

// promise runs run on its own goroutine, since it may block and a
// JavaScript callback must not, and settles with what it returns.
func promise(run func() (js.Value, error)) js.Value {
	executor := js.FuncOf(func(_ js.Value, p []js.Value) any {
		resolve, reject := p[0], p[1]
		go func() {
			v, err := run()
			var r rejection
			switch {
			case errors.As(err, &r):
				reject.Invoke(r.reason)
			case err != nil:
				reject.Invoke(js.Global().Get("Error").New(err.Error()))
			default:
				resolve.Invoke(v)
			}
		}()
		return nil
	})
	defer executor.Release()
	return js.Global().Get("Promise").New(executor)
}
