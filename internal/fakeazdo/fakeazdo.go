// Package fakeazdo is a fake Azure DevOps API for tests, as fakegh is a
// fake gh. It is an httptest server that follows a script of Answers and
// records each request, so a test sees what the CI Watcher asked Azure
// DevOps. The project URL it gives is over http://, which the project URL
// parser accepts for it.
package fakeazdo

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
)

// Answer is how the fake answers a request whose method is Method (any
// when empty) and whose path, below the server's root, starts with Path.
type Answer struct {
	Method string
	Path   string
	Status int // 200 when zero
	Header http.Header
	Body   string
}

// JSON is an Answer to method and path with status and v as its body.
func JSON(method, path string, status int, v any) Answer {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return Answer{Method: method, Path: path, Status: status, Body: string(b)}
}

// WithHeader is a with the header set.
func (a Answer) WithHeader(key, value string) Answer {
	h := a.Header.Clone()
	if h == nil {
		h = http.Header{}
	}
	h.Set(key, value)
	a.Header = h
	return a
}

// Request is one request the fake received.
type Request struct {
	Method string
	// Path is the request's path and Query its query, as received.
	Path  string
	Query map[string][]string
	// Authorization is the Authorization header, so a test sees the PAT
	// went as Basic auth.
	Authorization string
	Body          string
}

// Server is a running fake.
type Server struct {
	*httptest.Server
	mu       sync.Mutex
	answers  []Answer
	requests []Request
}

// ProjectPath is the path of the fake's project below its URL.
const ProjectPath = "/tfs/DefaultCollection/web"

// New starts a fake with answers, stopped when the test ends.
func New(t interface {
	Cleanup(func())
}, answers ...Answer) *Server {
	s := &Server{answers: answers}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.Close)
	return s
}

// ProjectURL is the project URL that reaches the fake.
func (s *Server) ProjectURL() string { return s.URL + ProjectPath }

// Script adds answers to the fake's script; the latest that matches wins.
func (s *Server) Script(answers ...Answer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.answers = append(s.answers, answers...)
}

// Requests is every request so far, oldest first.
func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.requests)
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	s.mu.Lock()
	s.requests = append(s.requests, Request{
		Method: r.Method, Path: r.URL.EscapedPath(), Query: r.URL.Query(),
		Authorization: r.Header.Get("Authorization"), Body: string(body),
	})
	var found *Answer
	for _, a := range slices.Backward(s.answers) {
		if (a.Method == "" || a.Method == r.Method) && strings.HasPrefix(r.URL.EscapedPath(), a.Path) {
			found = &a
			break
		}
	}
	s.mu.Unlock()
	if found == nil {
		// Unscripted: an empty list, as a project with no builds answers.
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"count":0,"value":[]}`)
		return
	}
	for k, vs := range found.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
	}
	if found.Status != 0 {
		w.WriteHeader(found.Status)
	}
	io.WriteString(w, found.Body)
}
