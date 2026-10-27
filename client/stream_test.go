// Copyright Kirto LLC 2026
// SPDX-License-Identifier: Apache-2.0

package client_test

import (
	"bufio"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"kirto.dev/apimachinery/client"
	"kirto.dev/apimachinery/httpwire"
	"kirto.dev/apimachinery/resource"
)

func TestStreamHandsBackTheOpenBody(t *testing.T) {
	more := make(chan struct{})
	c := newRaw(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/snapshot" || r.Header.Get("Authorization") != "Bearer t" ||
			r.Header.Get("If-Match") != `"2"` || r.Header.Get("Accept") != "application/octet-stream" {
			t.Errorf("%s %s %v", r.Method, r.URL.Path, r.Header)
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		io.WriteString(w, "first\n")
		w.(http.Flusher).Flush()
		<-more
		io.WriteString(w, "second\n")
	}))
	s, err := c.Stream(t.Context(), http.MethodGet, "snapshot", nil,
		client.WithPrecondition(resource.Precondition{Revision: 2}), client.WithHeader("Accept", "application/octet-stream"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Body.Close()
	if s.StatusCode != http.StatusOK || s.Header.Get("Content-Type") != "application/octet-stream" {
		t.Fatalf("%+v", s.Response)
	}
	// The first line arrives while the server still holds the second:
	// nothing waited for the whole answer.
	rd := bufio.NewReader(s.Body)
	if line, err := rd.ReadString('\n'); err != nil || line != "first\n" {
		t.Fatalf("%q %v", line, err)
	}
	close(more)
	if rest, err := io.ReadAll(rd); err != nil || string(rest) != "second\n" {
		t.Fatalf("%q %v", rest, err)
	}
}

func TestStreamRefusalIsTypedWithTheBodyClosed(t *testing.T) {
	c := newRaw(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Why", "gone")
		httpwire.WriteError(w, resource.ErrNotFound)
	}))
	s, err := c.Stream(t.Context(), http.MethodGet, "things/a", nil)
	var he *httpwire.Error
	if !errors.Is(err, resource.ErrNotFound) || !errors.As(err, &he) || he.HTTPStatus != http.StatusNotFound {
		t.Fatalf("%v", err)
	}
	if s == nil || s.Body != nil || s.StatusCode != http.StatusNotFound || s.Header.Get("X-Why") != "gone" {
		t.Fatalf("%+v", s)
	}
}

func TestStreamReadsABoundedErrorBody(t *testing.T) {
	c := newRaw(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		io.WriteString(w, strings.Repeat("x", 3*httpwire.MaxErrorBytes))
	}))
	_, err := c.Stream(t.Context(), http.MethodGet, "things/a", nil)
	var he *httpwire.Error
	if !errors.As(err, &he) || he.HTTPStatus != http.StatusBadGateway || len(he.Message) > httpwire.MaxErrorBytes {
		t.Fatalf("%d bytes: %.80v", len(he.Message), err)
	}
}

func TestStreamFollowsNoRedirect(t *testing.T) {
	hops := 0
	c := newRaw(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hops++
		http.Redirect(w, r, "/elsewhere", http.StatusFound)
	}))
	s, err := c.Stream(t.Context(), http.MethodGet, "things", nil)
	var he *httpwire.Error
	if !errors.As(err, &he) || he.HTTPStatus != http.StatusFound || hops != 1 || s.Body != nil {
		t.Fatalf("%v after %d hops", err, hops)
	}
}

func TestStreamCloseEndsTheRequest(t *testing.T) {
	gone := make(chan struct{})
	c := newRaw(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "open\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(gone)
	}))
	s, err := c.Stream(t.Context(), http.MethodGet, "things?watch=true", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bufio.NewReader(s.Body).ReadString('\n'); err != nil {
		t.Fatal(err)
	}
	if err := s.Body.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-gone:
	case <-time.After(5 * time.Second):
		t.Fatal("closing the body left the server's request open")
	}
}

func TestStreamRefusesAStoreFenceUnsent(t *testing.T) {
	sent := false
	c := newRaw(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { sent = true }))
	s, err := c.Stream(t.Context(), http.MethodGet, "things", nil, client.WithPrecondition(resource.Precondition{StoreRevision: 2}))
	if !errors.Is(err, resource.ErrInvalid) || sent || s == nil || s.StatusCode != 0 {
		t.Fatalf("%v, sent %v", err, sent)
	}
}

func TestDoSendsAReaderUnbuffered(t *testing.T) {
	got := make(chan string)
	c := newRaw(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/octet-stream" || r.ContentLength != -1 {
			t.Errorf("%v length %d", r.Header, r.ContentLength)
		}
		// The first part is here before the client has the second to
		// send: the body went out as it was read, not after it ended.
		first := make([]byte, len("part one;"))
		if _, err := io.ReadFull(r.Body, first); err != nil {
			t.Error(err)
		}
		got <- string(first)
		rest, _ := io.ReadAll(r.Body)
		httpwire.WriteJSON(w, http.StatusOK, map[string]string{"rest": string(rest)})
	}))
	pr, pw := io.Pipe()
	go func() {
		io.WriteString(pw, "part one;")
		select {
		case first := <-got:
			if first != "part one;" {
				t.Errorf("first %q", first)
			}
		case <-time.After(5 * time.Second):
			t.Error("the server saw nothing of a body still being written")
		}
		io.WriteString(pw, "part two")
		pw.Close()
	}()
	var out struct {
		Rest string `json:"rest"`
	}
	if _, err := c.Do(t.Context(), http.MethodPost, "cluster/snapshot", client.RawReader{ContentType: "application/octet-stream", Body: pr}, &out); err != nil {
		t.Fatal(err)
	}
	if out.Rest != "part two" {
		t.Fatalf("%+v", out)
	}
}

func TestStreamSendsAReaderBody(t *testing.T) {
	c := newRaw(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Header.Get("Content-Type") != "text/plain" || string(body) != "hello" {
			t.Errorf("%v %q", r.Header, body)
		}
		io.WriteString(w, "olleh")
	}))
	s, err := c.Stream(t.Context(), http.MethodPut, "echo", &client.RawReader{ContentType: "text/plain", Body: strings.NewReader("hello")})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Body.Close()
	if raw, err := io.ReadAll(s.Body); err != nil || string(raw) != "olleh" {
		t.Fatalf("%q %v", raw, err)
	}
}

func TestDoRefusesAnAnswerPastTheBound(t *testing.T) {
	c := newRaw(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chunk := strings.Repeat("x", 1<<20)
		for range client.MaxBodyBytes>>20 + 1 {
			io.WriteString(w, chunk)
		}
	}))
	if _, err := c.Do(t.Context(), http.MethodGet, "big", nil, nil); err == nil || !strings.Contains(err.Error(), "answered more than") {
		t.Fatalf("%v", err)
	}
}
