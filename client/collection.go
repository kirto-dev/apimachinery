// Copyright Kirto LLC 2026
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"net/http"
	"net/url"
	"strconv"

	"kirto.dev/apimachinery/httpwire"
	"kirto.dev/apimachinery/resource"
)

// Collection is one collection's objects, decoded as T. Its methods are
// plain, so a caller can name the ones it uses in an interface and fake
// them. The zero value has no client and fails every call.
type Collection[T any] struct {
	c    *Client
	path string
}

// ListOptions page a list. A server that does not page answers the whole
// collection and no continue token whatever Limit says, as the
// Kubernetes list contract allows.
type ListOptions struct {
	// Limit asks for at most this many items; 0 asks for all of them.
	Limit int
	// Continue is the previous page's [List.Continue], to read the page
	// after it.
	Continue string
}

// List is one page of a collection.
type List[T any] struct {
	Items []resource.Item[T]
	// ResourceVersion is the cursor of the snapshot the items were read
	// at, the [WatchOptions.ResourceVersion] that follows the collection
	// from this list on.
	ResourceVersion string
	// Continue reads the next page; empty on the last one.
	Continue string
}

// WatchOptions are where a watch starts and how long it stays open.
type WatchOptions struct {
	// ResourceVersion is the cursor to resume from: a list's or a
	// bookmark's. Empty starts from the current state, delivered as
	// Added events and a Bookmark.
	ResourceVersion string
	// TimeoutSeconds bounds the stream's span; 0 is the server's
	// longest, which a server holds every longer one to.
	TimeoutSeconds int
}

func (c Collection[T]) member(name string) string { return c.path + "/" + url.PathEscape(name) }

func (c Collection[T]) client() (*Client, error) {
	if c.c == nil {
		return nil, fmt.Errorf("client: a zero Collection has no client")
	}
	return c.c, nil
}

// Get reads the object name.
func (c Collection[T]) Get(ctx context.Context, name string) (resource.Item[T], error) {
	cl, err := c.client()
	if err != nil {
		return resource.Item[T]{}, err
	}
	raw, err := cl.read(ctx, http.MethodGet, c.member(name), nil, nil)
	if err != nil {
		return resource.Item[T]{}, err
	}
	return decodeItem[T](raw)
}

// List reads one page of the collection.
func (c Collection[T]) List(ctx context.Context, o ListOptions) (List[T], error) {
	cl, err := c.client()
	if err != nil {
		return List[T]{}, err
	}
	q := url.Values{}
	if o.Limit > 0 {
		q.Set("limit", strconv.Itoa(o.Limit))
	}
	if o.Continue != "" {
		q.Set("continue", o.Continue)
	}
	u := c.path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	raw, err := cl.read(ctx, http.MethodGet, u, nil, nil)
	if err != nil {
		return List[T]{}, err
	}
	var l struct {
		Metadata struct {
			ResourceVersion string `json:"resourceVersion"`
			Continue        string `json:"continue"`
		} `json:"metadata"`
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(raw, &l); err != nil {
		return List[T]{}, fmt.Errorf("client: decoding the list of %s: %w", c.path, err)
	}
	out := List[T]{Items: make([]resource.Item[T], 0, len(l.Items)), ResourceVersion: l.Metadata.ResourceVersion, Continue: l.Metadata.Continue}
	for _, it := range l.Items {
		item, err := decodeItem[T](it)
		if err != nil {
			return List[T]{}, err
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}

// Put writes v as the object name under pre, the compare-and-swap:
// Revision 0 creates, sent as If-None-Match: *, and any other revision
// replaces exactly that one, sent as If-Match. A stale revision is a
// conflict, errors.Is(err, resource.ErrConflict). pre.StoreRevision
// fences a store-wide listing, which no HTTP header carries, so a
// non-zero one is refused unsent.
func (c Collection[T]) Put(ctx context.Context, name string, v T, pre resource.Precondition) (resource.Item[T], error) {
	cl, err := c.client()
	if err != nil {
		return resource.Item[T]{}, err
	}
	if pre.StoreRevision != 0 {
		return resource.Item[T]{}, &resource.InvalidError{Reason: "a store revision fence has no HTTP header; send the object's revision alone"}
	}
	raw, err := cl.read(ctx, http.MethodPut, c.member(name), v, func(h http.Header) { httpwire.SetPrecondition(h, pre) })
	if err != nil {
		return resource.Item[T]{}, err
	}
	return decodeItem[T](raw)
}

// Create writes v as the object name, which must not exist yet: [Put]
// at Revision 0. An object already there is a conflict.
func (c Collection[T]) Create(ctx context.Context, name string, v T) (resource.Item[T], error) {
	return c.Put(ctx, name, v, resource.Precondition{})
}

// Delete removes the object name at pre.Revision, the revision it was
// read at, sent as If-Match. A delete always names one: Revision 0 is
// sent with no precondition, and the server refuses it.
func (c Collection[T]) Delete(ctx context.Context, name string, pre resource.Precondition) error {
	cl, err := c.client()
	if err != nil {
		return err
	}
	if pre.StoreRevision != 0 {
		return &resource.InvalidError{Reason: "a store revision fence has no HTTP header; send the object's revision alone"}
	}
	set := func(h http.Header) {
		if pre.Revision != 0 {
			httpwire.SetPrecondition(h, pre)
		}
	}
	_, err = cl.read(ctx, http.MethodDelete, c.member(name), nil, set)
	return err
}

// Watch follows the collection. The stream opens when the sequence is
// first pulled. A refusal before it opens is the sequence's only
// element, as an error: a cursor too old is errors.Is(err,
// resource.ErrTooOld), and the caller lists again. Each line is then one
// event: Added and Modified carry the object, Deleted its metadata
// alone, Bookmark the cursor to resume from. An ERROR line ends the
// sequence with its typed error. The end of the stream's span ends the
// sequence cleanly, and the caller watches again from its last
// bookmark. Stopping the range closes the stream.
func (c Collection[T]) Watch(ctx context.Context, o WatchOptions) iter.Seq2[resource.Projected[T], error] {
	return func(yield func(resource.Projected[T], error) bool) {
		cl, err := c.client()
		if err != nil {
			yield(resource.Projected[T]{}, err)
			return
		}
		q := url.Values{"watch": {"true"}}
		if o.ResourceVersion != "" {
			q.Set("resourceVersion", o.ResourceVersion)
		}
		if o.TimeoutSeconds > 0 {
			q.Set("timeoutSeconds", strconv.Itoa(o.TimeoutSeconds))
		}
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		resp, err := cl.do(ctx, http.MethodGet, c.path+"?"+q.Encode(), nil, nil)
		if err != nil {
			yield(resource.Projected[T]{}, err)
			return
		}
		defer resp.Body.Close()
		for ev, err := range httpwire.DecodeWatch(resp.Body) {
			if err != nil {
				if ctx.Err() == nil {
					yield(resource.Projected[T]{}, err)
				}
				return
			}
			p, err := project[T](ev)
			if err != nil {
				yield(resource.Projected[T]{}, err)
				return
			}
			if !yield(p, nil) {
				return
			}
		}
	}
}

// project decodes one watch line into a typed event, or the line's
// error.
func project[T any](ev httpwire.Event) (resource.Projected[T], error) {
	raw, _ := ev.Object.(json.RawMessage)
	switch ev.Type {
	case httpwire.EventAdded, httpwire.EventModified:
		it, err := decodeItem[T](raw)
		if err != nil {
			return resource.Projected[T]{}, err
		}
		typ := resource.Added
		if ev.Type == httpwire.EventModified {
			typ = resource.Modified
		}
		return resource.Projected[T]{Type: typ, Meta: it.Meta, Value: it.Value}, nil
	case httpwire.EventDeleted:
		m, err := decodeMeta(raw)
		if err != nil {
			return resource.Projected[T]{}, err
		}
		return resource.Projected[T]{Type: resource.Deleted, Meta: m}, nil
	case httpwire.EventBookmark:
		m, err := decodeMeta(raw)
		if err != nil {
			return resource.Projected[T]{}, err
		}
		return resource.Projected[T]{Type: resource.Bookmark, Cursor: m.ResourceVersion}, nil
	case httpwire.EventError:
		var s httpwire.Status
		if err := json.Unmarshal(raw, &s); err != nil || s.Code == "" {
			return resource.Projected[T]{}, fmt.Errorf("client: an ERROR line %s that is not a Status", raw)
		}
		return resource.Projected[T]{}, httpwire.ErrorFrom(s, httpwire.HTTPStatus(s.Code))
	}
	return resource.Projected[T]{}, fmt.Errorf("client: a watch line of type %q", ev.Type)
}

func decodeMeta(raw []byte) (resource.Meta, error) {
	var o struct {
		Metadata resource.Meta `json:"metadata"`
	}
	if err := json.Unmarshal(raw, &o); err != nil {
		return resource.Meta{}, fmt.Errorf("client: decoding an object's metadata: %w", err)
	}
	return o.Metadata, nil
}

// decodeItem is an object answer as its metadata and its value.
func decodeItem[T any](raw []byte) (resource.Item[T], error) {
	m, err := decodeMeta(raw)
	if err != nil {
		return resource.Item[T]{}, err
	}
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		return resource.Item[T]{}, fmt.Errorf("client: decoding an object: %w", err)
	}
	return resource.Item[T]{Meta: m, Value: v}, nil
}
