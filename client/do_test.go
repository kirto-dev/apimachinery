// Copyright Kirto LLC 2026
// SPDX-License-Identifier: Apache-2.0

package client_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"kirto.dev/apimachinery/client"
	"kirto.dev/apimachinery/httpwire"
	"kirto.dev/apimachinery/resource"
)

func newRaw(t *testing.T, h http.Handler) *client.Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := client.New(srv.URL+"/v1", client.Options{
		HTTPClient: srv.Client(),
		Header: func(_ context.Context, h http.Header) error {
			h.Set("Authorization", "Bearer t")
			h.Set("X-Trace", "hook")
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestDoSendsJSONAndReadsTheAnswer(t *testing.T) {
	c := newRaw(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Method != http.MethodPost || r.URL.Path != "/v1/models/a/activate" || r.URL.RawQuery != "dry=1" ||
			r.Header.Get("Authorization") != "Bearer t" || r.Header.Get("If-Match") != `"4"` ||
			r.Header.Get("Content-Type") != "application/json" || string(body) != `{"version":"v1"}` ||
			r.Header.Get("X-Trace") != "call" {
			t.Errorf("%s %s?%s %v %s", r.Method, r.URL.Path, r.URL.RawQuery, r.Header, body)
		}
		httpwire.SetETag(w, 5)
		httpwire.WriteJSON(w, http.StatusCreated, map[string]string{"version": "v1"})
	}))
	var out struct {
		Version string `json:"version"`
	}
	ans, err := c.Do(t.Context(), http.MethodPost, "/models/a/activate?dry=1", map[string]string{"version": "v1"}, &out,
		client.WithPrecondition(resource.Precondition{Revision: 4}), client.WithHeader("X-Trace", "call"))
	if err != nil || out.Version != "v1" || ans.StatusCode != http.StatusCreated {
		t.Fatalf("%+v %+v %v", ans, out, err)
	}
	if rev, err := ans.Revision(); err != nil || rev != 5 {
		t.Fatalf("revision %d %v", rev, err)
	}
}

func TestDoSendsAndReadsRawBodies(t *testing.T) {
	c := newRaw(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Header.Get("Content-Type") != "application/yaml" || string(body) != "a: 1\n" || r.Header.Get("If-None-Match") != "*" {
			t.Errorf("%v %q", r.Header, body)
		}
		w.Header().Set("Content-Type", "application/yaml")
		io.WriteString(w, "b: 2\n")
	}))
	var out client.Raw
	if _, err := c.Do(t.Context(), http.MethodPut, "sources/a", client.Raw{ContentType: "application/yaml", Data: []byte("a: 1\n")}, &out,
		client.WithPrecondition(resource.Precondition{})); err != nil {
		t.Fatal(err)
	}
	if out.ContentType != "application/yaml" || string(out.Data) != "b: 2\n" {
		t.Fatalf("%+v", out)
	}
}

func TestDoRefusalsAreTypedAndKeepTheAnswer(t *testing.T) {
	c := newRaw(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Why", "stale")
		httpwire.WriteError(w, &resource.ConflictError{Key: resource.Key{Kind: "thing", Name: "a"}, Expected: 3, Current: 5})
	}))
	ans, err := c.Do(t.Context(), http.MethodDelete, "things/a", nil, nil, client.WithPrecondition(resource.Precondition{Revision: 3}))
	var ce *resource.ConflictError
	var he *httpwire.Error
	if !errors.As(err, &ce) || ce.Current != 5 || !errors.As(err, &he) || he.HTTPStatus != http.StatusPreconditionFailed {
		t.Fatalf("%v", err)
	}
	if ans.StatusCode != http.StatusPreconditionFailed || ans.Header.Get("X-Why") != "stale" {
		t.Fatalf("%+v", ans)
	}
}

func TestDoFollowsNoRedirect(t *testing.T) {
	hops := 0
	c := newRaw(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hops++
		http.Redirect(w, r, "/elsewhere", http.StatusPermanentRedirect)
	}))
	ans, err := c.Do(t.Context(), http.MethodPut, "things/a", map[string]string{"secret": "s"}, nil)
	var he *httpwire.Error
	if !errors.As(err, &he) || he.HTTPStatus != http.StatusPermanentRedirect || hops != 1 || ans.StatusCode != http.StatusPermanentRedirect {
		t.Fatalf("%v after %d hops", err, hops)
	}
}

func TestDoRefusesAStoreFenceUnsent(t *testing.T) {
	sent := false
	c := newRaw(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { sent = true }))
	_, err := c.Do(t.Context(), http.MethodPut, "things/a", nil, nil, client.WithPrecondition(resource.Precondition{Revision: 1, StoreRevision: 2}))
	if !errors.Is(err, resource.ErrInvalid) || sent {
		t.Fatalf("%v, sent %v", err, sent)
	}
}

func TestRevisionNeedsAnETag(t *testing.T) {
	if _, err := (client.Response{Header: http.Header{}}).Revision(); err == nil {
		t.Fatal("no ETag")
	}
	if _, err := (client.Response{Header: http.Header{"Etag": {"abc"}}}).Revision(); err == nil {
		t.Fatal("a tag that is no revision")
	}
}
