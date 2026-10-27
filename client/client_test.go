// Copyright Kirto LLC 2026
// SPDX-License-Identifier: Apache-2.0

package client_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kirto.dev/apimachinery/client"
	"kirto.dev/apimachinery/httpwire"
	"kirto.dev/apimachinery/resource"
)

type thing struct {
	Metadata resource.Meta `json:"metadata"`
	Value    string        `json:"value"`
}

// things is what a caller names of a collection, so a test can fake it.
type things interface {
	Get(ctx context.Context, name string) (resource.Item[thing], error)
	List(ctx context.Context, o client.ListOptions) (client.List[thing], error)
	Put(ctx context.Context, name string, v thing, pre resource.Precondition) (resource.Item[thing], error)
	Create(ctx context.Context, name string, v thing) (resource.Item[thing], error)
	Delete(ctx context.Context, name string, pre resource.Precondition) error
	Watch(ctx context.Context, o client.WatchOptions) iter.Seq2[resource.Projected[thing], error]
}

var _ things = client.Collection[thing]{}

func newClient(t *testing.T, h http.Handler) things {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := client.New(srv.URL+"/v1/", client.Options{
		HTTPClient: srv.Client(),
		Header: func(_ context.Context, h http.Header) error {
			h.Set("Authorization", "Bearer t")
			h.Set("X-Namespace", "team")
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return c.Collection[thing]("things")
}

func TestNewRefusesARelativeURL(t *testing.T) {
	if _, err := client.New("/v1", client.Options{}); err == nil {
		t.Fatal("a relative base URL")
	}
}

func TestGetSendsTheHookHeadersAndDecodes(t *testing.T) {
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/v1/things/a%2Fb" || r.Header.Get("Authorization") != "Bearer t" || r.Header.Get("X-Namespace") != "team" {
			t.Errorf("%s %s %v", r.Method, r.URL.EscapedPath(), r.Header)
		}
		httpwire.SetETag(w, 4)
		httpwire.WriteJSON(w, 200, thing{Metadata: resource.Meta{Name: "a/b", ResourceVersion: "4"}, Value: "v"})
	}))
	it, err := c.Get(t.Context(), "a/b")
	if err != nil || it.Meta.ResourceVersion != "4" || it.Value.Value != "v" || it.Value.Metadata.Name != "a/b" {
		t.Fatalf("%+v %v", it, err)
	}
}

func TestErrorsAreTyped(t *testing.T) {
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			httpwire.WriteError(w, &resource.NotFoundError{Key: resource.Key{Kind: "thing", Name: "a"}})
		default:
			httpwire.WriteError(w, &resource.ConflictError{Key: resource.Key{Kind: "thing", Name: "a"}, Expected: 3, Current: 5})
		}
	}))
	if _, err := c.Get(t.Context(), "a"); !errors.Is(err, resource.ErrNotFound) {
		t.Fatalf("get = %v, want not found", err)
	}
	_, err := c.Put(t.Context(), "a", thing{}, resource.Precondition{Revision: 3})
	var ce *resource.ConflictError
	var he *httpwire.Error
	if !errors.As(err, &ce) || ce.Current != 5 || !errors.As(err, &he) || he.HTTPStatus != 412 {
		t.Fatalf("put = %v", err)
	}
}

func TestWritesSendTheirPrecondition(t *testing.T) {
	var got []string
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Method+" "+r.Header.Get("If-Match")+"|"+r.Header.Get("If-None-Match"))
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		var body thing
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("body %+v %v", body, err)
		}
		httpwire.WriteJSON(w, 200, thing{Metadata: resource.Meta{Name: "a", ResourceVersion: "9"}, Value: body.Value})
	}))
	if it, err := c.Create(t.Context(), "a", thing{Value: "x"}); err != nil || it.Value.Value != "x" || it.Meta.ResourceVersion != "9" {
		t.Fatalf("create %+v %v", it, err)
	}
	if _, err := c.Put(t.Context(), "a", thing{Value: "y"}, resource.Precondition{Revision: 9}); err != nil {
		t.Fatal(err)
	}
	if err := c.Delete(t.Context(), "a", resource.Precondition{Revision: 10}); err != nil {
		t.Fatal(err)
	}
	if want := []string{`PUT |*`, `PUT "9"|`, `DELETE "10"|`}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("%q, want %q", got, want)
	}
	if _, err := c.Put(t.Context(), "a", thing{}, resource.Precondition{Revision: 1, StoreRevision: 2}); !errors.Is(err, resource.ErrInvalid) || len(got) != 3 {
		t.Fatalf("a store fence = %v, sent %d", err, len(got))
	}
}

func TestListPages(t *testing.T) {
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("continue") == "" {
			if r.URL.Query().Get("limit") != "1" {
				t.Errorf("query %s", r.URL.RawQuery)
			}
			fmt.Fprint(w, `{"metadata":{"resourceVersion":"7.e","continue":"next"},"items":[{"metadata":{"name":"a","resourceVersion":"3"},"value":"1"}]}`)
			return
		}
		fmt.Fprint(w, `{"metadata":{"resourceVersion":"7.e"},"items":[{"metadata":{"name":"b","resourceVersion":"5"},"value":"2"}]}`)
	}))
	var names []string
	o := client.ListOptions{Limit: 1}
	for {
		l, err := c.List(t.Context(), o)
		if err != nil || l.ResourceVersion != "7.e" {
			t.Fatalf("%+v %v", l, err)
		}
		for _, it := range l.Items {
			names = append(names, it.Meta.Name+"="+it.Value.Value)
		}
		if l.Continue == "" {
			break
		}
		o.Continue = l.Continue
	}
	if fmt.Sprint(names) != "[a=1 b=2]" {
		t.Fatalf("%v", names)
	}
}

func TestARedirectIsAnErrorNotAHop(t *testing.T) {
	hops := 0
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hops++
		http.Redirect(w, r, "/elsewhere", http.StatusTemporaryRedirect)
	}))
	_, err := c.Put(t.Context(), "a", thing{}, resource.Precondition{})
	var he *httpwire.Error
	if !errors.As(err, &he) || he.HTTPStatus != http.StatusTemporaryRedirect || !strings.Contains(he.Message, "/elsewhere") || hops != 1 {
		t.Fatalf("%v after %d hops", err, hops)
	}
}

func TestWatch(t *testing.T) {
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("watch") != "true" {
			t.Errorf("query %s", r.URL.RawQuery)
		}
		if q.Get("resourceVersion") == "1.old" {
			httpwire.WriteError(w, &resource.TooOldError{Revision: 1, Epoch: "e"})
			return
		}
		if q.Get("resourceVersion") != "4.e" || q.Get("timeoutSeconds") != "5" {
			t.Errorf("query %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", httpwire.ContentTypeWatch)
		for _, l := range []string{
			`{"type":"BOOKMARK","object":{"metadata":{"resourceVersion":"4.e"}}}`,
			`{"type":"ADDED","object":{"metadata":{"name":"a","resourceVersion":"5"},"value":"1"}}`,
			`{"type":"MODIFIED","object":{"metadata":{"name":"a","resourceVersion":"6"},"value":"2"}}`,
			`{"type":"DELETED","object":{"metadata":{"name":"a","resourceVersion":"7"}}}`,
			`{"type":"ERROR","object":{"code":"too_old","message":"gone","revision":7,"epoch":"f"}}`,
		} {
			fmt.Fprintln(w, l)
		}
	}))
	var got []string
	var end error
	for ev, err := range c.Watch(t.Context(), client.WatchOptions{ResourceVersion: "4.e", TimeoutSeconds: 5}) {
		if err != nil {
			end = err
			break
		}
		got = append(got, fmt.Sprintf("%s %s %s %s %s", ev.Type, ev.Meta.Name, ev.Meta.ResourceVersion, ev.Value.Value, ev.Cursor))
	}
	want := []string{"bookmark    4.e", "added a 5 1 ", "modified a 6 2 ", "deleted a 7  "}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("%q, want %q", got, want)
	}
	var to *resource.TooOldError
	if !errors.As(end, &to) || to.Epoch != "f" {
		t.Fatalf("the ERROR line ended with %v", end)
	}

	n := 0
	for _, err := range c.Watch(t.Context(), client.WatchOptions{ResourceVersion: "1.old"}) {
		n++
		var he *httpwire.Error
		if !errors.Is(err, resource.ErrTooOld) || !errors.As(err, &he) || he.HTTPStatus != http.StatusGone {
			t.Fatalf("a refused watch = %v", err)
		}
	}
	if n != 1 {
		t.Fatalf("a refused watch yielded %d elements", n)
	}
}

func TestAZeroCollectionFails(t *testing.T) {
	var c client.Collection[thing]
	if _, err := c.Get(context.Background(), "a"); err == nil {
		t.Fatal("a zero collection read")
	}
}
