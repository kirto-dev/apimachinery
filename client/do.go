// Copyright Kirto LLC 2026
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"kirto.dev/apimachinery/httpwire"
	"kirto.dev/apimachinery/resource"
)

// Raw is a body sent or read as its bytes rather than as JSON: a
// document whose bytes are the point, such as a YAML source or an XML
// model. As [Client.Do]'s in it is sent with its ContentType; as its
// out, a *Raw, it is filled with the answer's Content-Type and body.
type Raw struct {
	ContentType string
	Data        []byte
}

// Response is what an answer to [Client.Do] carries beside its body.
type Response struct {
	// StatusCode is the HTTP status; 0 when no answer arrived.
	StatusCode int
	// Header is the answer's headers; nil when no answer arrived.
	Header http.Header
}

// Revision is the revision the answer's ETag names: what a write of the
// object it describes sends next as its precondition. An answer with no
// ETag, or one that names no revision, is an error.
func (r Response) Revision() (uint64, error) {
	tag := r.Header.Get("ETag")
	if tag == "" {
		return 0, fmt.Errorf("client: the answer carries no ETag")
	}
	pre, err := httpwire.ParsePrecondition(tag, "", false)
	if err != nil {
		return 0, fmt.Errorf("client: ETag %s: %w", tag, err)
	}
	return pre.Revision, nil
}

// CallOption shapes one [Client.Do] request.
type CallOption func(*call)

type call struct {
	pre    *resource.Precondition
	header http.Header
}

// WithPrecondition sends pre as the write's conditional header, as
// [Collection.Put] does: Revision 0 is If-None-Match: *, any other
// If-Match with its ETag. A non-zero pre.StoreRevision has no header,
// and the request is refused unsent.
func WithPrecondition(pre resource.Precondition) CallOption {
	return func(c *call) { c.pre = &pre }
}

// WithHeader sets the request header key to value, after the client's
// own headers, so it wins over them.
func WithHeader(key, value string) CallOption {
	return func(c *call) {
		if c.header == nil {
			c.header = http.Header{}
		}
		c.header.Set(key, value)
	}
}

// Do sends one request that is not a collection's own: a verb such as
// an activation, a singleton such as the caller's identity, a write
// whose body is not the object it answers. path is below the base URL,
// its segments already escaped, and may carry a query.
//
// in is sent as JSON, or as it is when it is a [Raw]; nil sends no body.
// A JSON answer is decoded into out; a *Raw out takes the body as it is,
// and nil discards it. Do uses the client's HTTP client, its header hook
// and its redirect policy, so a redirect is an error, never a hop. A
// refusal is an *httpwire.Error that unwraps to the typed error of
// package resource its code names, and the Response beside it still
// carries the status and headers of the answer.
func (c *Client) Do(ctx context.Context, method, path string, in, out any, opts ...CallOption) (Response, error) {
	var o call
	for _, opt := range opts {
		opt(&o)
	}
	if o.pre != nil && o.pre.StoreRevision != 0 {
		return Response{}, &resource.InvalidError{Reason: "a store revision fence has no HTTP header; send the object's revision alone"}
	}
	set := func(h http.Header) {
		if o.pre != nil {
			httpwire.SetPrecondition(h, *o.pre)
		}
		for k, v := range o.header {
			h[k] = v
		}
	}
	resp, raw, err := c.readAll(ctx, method, c.base+"/"+strings.TrimLeft(path, "/"), in, set)
	var r Response
	if resp != nil {
		r = Response{StatusCode: resp.StatusCode, Header: resp.Header}
	}
	if err != nil {
		return r, err
	}
	switch out := out.(type) {
	case nil:
	case *Raw:
		out.ContentType, out.Data = resp.Header.Get("Content-Type"), raw
	default:
		if len(bytes.TrimSpace(raw)) == 0 {
			break
		}
		if err := json.Unmarshal(raw, out); err != nil {
			return r, fmt.Errorf("client: %s %s: decoding the answer: %w", method, path, err)
		}
	}
	return r, nil
}
