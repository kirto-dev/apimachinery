// Copyright Kirto LLC 2026
// SPDX-License-Identifier: Apache-2.0

// Package client is a typed client for resource routes on the wire
// contract of packages resource and httpwire.
//
// A [Client] holds the server's base URL, the HTTP client and a hook
// that sets each request's headers: the bearer, a namespace. Its one
// generic method, [Client.Collection], names a collection and the Go
// type of its objects, and the [Collection] it returns has plain
// methods, so a caller can put an ordinary interface in front of it and
// fake the collection in a test:
//
//	c, err := client.New("https://flow.example/v1", client.Options{Header: bearer})
//	things := c.Collection[Thing]("things")
//	it, err := things.Get(ctx, "a")
//	it, err = things.Put(ctx, "a", it.Value, resource.Precondition{Revision: rev})
//	for ev, err := range things.Watch(ctx, client.WatchOptions{ResourceVersion: list.ResourceVersion}) {
//		...
//	}
//
// Every refusal is an *httpwire.Error that unwraps to the typed error
// of package resource its code names, so errors.Is(err,
// resource.ErrConflict) holds for a stale write. The client never
// follows a redirect.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"kirto.dev/apimachinery/httpwire"
)

// Options configures a [Client].
type Options struct {
	// HTTPClient sends every request. The client uses a copy whose
	// CheckRedirect refuses every redirect. Nil is a client with no
	// timeout: a request is bounded by its context, and a watch by its
	// span, which a client timeout would cut short.
	HTTPClient *http.Client
	// Header sets a request's headers before it is sent: the bearer, a
	// namespace header. It runs on every request, so a token read from a
	// file that rotates is read fresh. An error it returns fails the
	// request unsent. Nil sets nothing.
	Header func(ctx context.Context, h http.Header) error
}

// Client speaks to one server's resource routes.
type Client struct {
	base   string
	http   *http.Client
	header func(context.Context, http.Header) error
}

// New is a client for the routes below baseURL, an absolute URL such as
// "https://flow.example/v1".
func New(baseURL string, o Options) (*Client, error) {
	u, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("client: base url %q is not an absolute URL", baseURL)
	}
	hc := &http.Client{}
	if o.HTTPClient != nil {
		c := *o.HTTPClient
		hc = &c
	}
	// A redirect is answered, never followed: Go keeps Authorization on
	// a same-host hop, a scheme or port change included, and a 307 or
	// 308 replays a PUT, body and all, to wherever it points.
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{base: u.String(), http: hc, header: o.Header}, nil
}

// Collection is the collection at kind, the path below the base URL
// that lists it ("things", or "admin/policies"), with objects decoded as
// T. T is the object as the route answers it, its metadata block
// included or not: the client reads the metadata itself.
func (c *Client) Collection[T any](kind string) Collection[T] {
	return Collection[T]{c: c, path: c.base + "/" + strings.Trim(kind, "/")}
}

// MaxBodyBytes bounds an answer the client reads whole: an object or a
// list.
const MaxBodyBytes = 64 << 20

// do sends one request and returns the answer of a status below 300,
// whose body the caller closes. Any other answer is its
// [httpwire.ReadError].
func (c *Client) do(ctx context.Context, method, u string, in any, set func(http.Header)) (*http.Response, error) {
	var body io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return nil, fmt.Errorf("client: encoding the %s body: %w", method, err)
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if set != nil {
		set(req.Header)
	}
	if c.header != nil {
		if err := c.header(ctx, req.Header); err != nil {
			return nil, err
		}
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if err := httpwire.ReadError(resp); err != nil {
		resp.Body.Close()
		return nil, err
	}
	return resp, nil
}

// read sends one request and returns its whole answer.
func (c *Client) read(ctx context.Context, method, u string, in any, set func(http.Header)) ([]byte, error) {
	resp, err := c.do(ctx, method, u, in, set)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, MaxBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > MaxBodyBytes {
		return nil, fmt.Errorf("client: %s %s answered more than %d bytes", method, u, MaxBodyBytes)
	}
	return raw, nil
}
