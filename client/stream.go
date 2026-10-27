// Copyright Kirto LLC 2026
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"kirto.dev/apimachinery/httpwire"
	"kirto.dev/apimachinery/resource"
)

// RawReader is a body sent as it is read rather than held whole: an
// upload such as a snapshot, whose size the client neither knows nor
// bounds. It is [Raw] for a reader. As the in of [Client.Do] or
// [Client.Stream] it is sent with its ContentType, read from Body while
// the request is written; a nil Body sends none. The client does not
// close Body.
type RawReader struct {
	ContentType string
	Body        io.Reader
}

// Stream is an answer whose body is read as it arrives: a download, a
// watch, anything too large or too long to hold whole. The caller
// closes Body.
type Stream struct {
	Response
	// Body is the answer's body, open; nil beside an error.
	Body io.ReadCloser
}

// Stream sends one request as [Client.Do] does, over the same header
// hook, options, redirect policy and refusal decoding, and returns the
// answer with its body open rather than read. path is below the base
// URL, its segments already escaped, and may carry a query.
//
// Only an answer below 300 has an open Body. A refusal is the error,
// typed as Do's is, with its body read (at most [httpwire.MaxErrorBytes]
// of it) and closed; the Stream beside any error is non-nil, its Body
// nil, its Response the status and headers of the answer, or zero when
// none arrived.
func (c *Client) Stream(ctx context.Context, method, path string, in any, opts ...CallOption) (*Stream, error) {
	var o call
	for _, opt := range opts {
		opt(&o)
	}
	if o.pre != nil && o.pre.StoreRevision != 0 {
		return &Stream{}, &resource.InvalidError{Reason: "a store revision fence has no HTTP header; send the object's revision alone"}
	}
	body, contentType, err := encode(method, in)
	if err != nil {
		return &Stream{}, err
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+"/"+strings.TrimLeft(path, "/"), body)
	if err != nil {
		return &Stream{}, err
	}
	req.Header.Set("Accept", "application/json")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if c.header != nil {
		if err := c.header(ctx, req.Header); err != nil {
			return &Stream{}, err
		}
	}
	if o.pre != nil {
		httpwire.SetPrecondition(req.Header, *o.pre)
	}
	for k, v := range o.header {
		req.Header[k] = v
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return &Stream{}, err
	}
	s := &Stream{Response: Response{StatusCode: resp.StatusCode, Header: resp.Header}}
	if err := httpwire.ReadError(resp); err != nil {
		resp.Body.Close()
		return s, err
	}
	s.Body = resp.Body
	return s, nil
}

// encode is in as a request body and its Content-Type: none for nil,
// as it is for a [Raw] or a [RawReader], JSON for anything else.
func encode(method string, in any) (io.Reader, string, error) {
	switch v := in.(type) {
	case nil:
		return nil, "", nil
	case Raw:
		return bytes.NewReader(v.Data), v.ContentType, nil
	case *Raw:
		return bytes.NewReader(v.Data), v.ContentType, nil
	case RawReader:
		return v.Body, v.ContentType, nil
	case *RawReader:
		return v.Body, v.ContentType, nil
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return nil, "", fmt.Errorf("client: encoding the %s body: %w", method, err)
	}
	return bytes.NewReader(raw), "application/json", nil
}
