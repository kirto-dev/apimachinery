// Copyright Kirto LLC 2026
// SPDX-License-Identifier: Apache-2.0

package httpwire

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kirto.dev/apimachinery/resource"
)

func TestHTTPStatusTable(t *testing.T) {
	for code, want := range map[resource.Code]int{
		resource.CodeConflict:    412,
		CodePreconditionRequired: 428,
		resource.CodeSetConflict: 409,
		CodeAlreadyExists:        409,
		resource.CodeTooOld:      410,
		resource.CodeNotFound:    404,
		resource.CodeUnknownKind: 404,
		resource.CodeInvalid:     422,
		CodeBadRequest:           400,
		resource.CodeInvalidName: 400,
		resource.CodeTooLarge:    413,
		resource.CodeQuota:       409,
		CodeUnauthorized:         401,
		CodeForbidden:            403,
		CodeMethodNotAllowed:     405,
		CodeTooManyRequests:      429,
		CodeInternal:             500,
		resource.CodeUnsupported: 501,
		CodeUnavailable:          503,
		"a_product_code":         500,
	} {
		if got := HTTPStatus(code); got != want {
			t.Errorf("HTTPStatus(%s) = %d, want %d", code, got, want)
		}
	}
}

func TestGenericCode(t *testing.T) {
	for status, want := range map[int]resource.Code{
		400: CodeBadRequest, 401: CodeUnauthorized, 403: CodeForbidden, 404: resource.CodeNotFound,
		405: CodeMethodNotAllowed, 409: CodeAlreadyExists, 410: resource.CodeTooOld,
		412: resource.CodeConflict, 413: resource.CodeTooLarge, 422: resource.CodeInvalid,
		428: CodePreconditionRequired, 429: CodeTooManyRequests, 500: CodeInternal,
		501: resource.CodeUnsupported, 503: CodeUnavailable,
		418: CodeBadRequest, 502: CodeInternal,
	} {
		if got := GenericCode(status); got != want {
			t.Errorf("GenericCode(%d) = %s, want %s", status, got, want)
		}
	}
	if e := Errorf(404, "", "no %s", "such"); e.Code != resource.CodeNotFound || e.Message != "no such" || e.HTTPStatus != 404 {
		t.Errorf("Errorf with no code = %+v", e)
	}
}

func TestFromError(t *testing.T) {
	k := resource.Key{Kind: "item", Namespace: "n", Name: "a"}
	err := fmt.Errorf("writing: %w", &resource.ConflictError{Key: k, Expected: 4182, Current: 4190})
	e := FromError(err)
	if e.HTTPStatus != 412 || e.Code != resource.CodeConflict || e.Expected != 4182 || e.Current != 4190 || *e.Key != k || e.Message != err.Error() {
		t.Fatalf("wrapped conflict = %+v", e)
	}
	if e := FromError(fmt.Errorf("x: %w", resource.ErrUnknownKind)); e.HTTPStatus != 404 || e.Code != resource.CodeUnknownKind {
		t.Fatalf("unknown kind = %+v", e)
	}
	own := Errorf(409, "overlap", "two sources declare one name")
	if got := FromError(fmt.Errorf("compose: %w", own)); got != own {
		t.Fatalf("a wrapped *Error = %+v, want it passed through", got)
	}
	if e := FromError(errors.New("disk on fire")); e.HTTPStatus != 500 || e.Code != CodeInternal || e.Message != "disk on fire" {
		t.Fatalf("a plain error = %+v", e)
	}
	if FromError(nil) != nil {
		t.Fatal("FromError(nil) is not nil")
	}
}

func TestErrorJSONIsTheStatus(t *testing.T) {
	e := &Error{
		Status:     Status{Status: resource.Status{Code: resource.CodeConflict, Message: "m", Expected: 3}, Causes: []Cause{{Field: "source", Message: "required"}}},
		HTTPStatus: 412,
	}
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"code":"conflict","message":"m","expected":3,"causes":[{"field":"source","message":"required"}]}`; string(b) != want {
		t.Fatalf("marshal %s, want %s", b, want)
	}
	var back Status
	if err := json.Unmarshal(b, &back); err != nil || back.Code != resource.CodeConflict || back.Expected != 3 || len(back.Causes) != 1 {
		t.Fatalf("decode %+v %v", back, err)
	}
	// A framework's status error by shape, with no import of it.
	var se interface {
		GetStatus() int
		Error() string
	} = e
	if se.GetStatus() != 412 || se.Error() != "m" {
		t.Fatalf("status error %d %q", se.GetStatus(), se.Error())
	}
}

func TestWriteError(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, &resource.NotFoundError{Key: resource.Key{Kind: "item", Name: "a"}})
	if rec.Code != http.StatusNotFound || rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("%d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	var s Status
	if err := json.Unmarshal(rec.Body.Bytes(), &s); err != nil || s.Code != resource.CodeNotFound || s.Key.Name != "a" {
		t.Fatalf("body %s: %+v %v", rec.Body, s, err)
	}
}

func answer(status int, contentType, body string, hdr ...string) *http.Response {
	h := http.Header{"Content-Type": {contentType}}
	for i := 0; i+1 < len(hdr); i += 2 {
		h.Set(hdr[i], hdr[i+1])
	}
	req := httptest.NewRequest(http.MethodPut, "https://user:secret@node.example/v1/things/a", nil)
	return &http.Response{StatusCode: status, Header: h, Body: io.NopCloser(strings.NewReader(body)), Request: req}
}

func TestReadError(t *testing.T) {
	if ReadError(answer(200, "application/json", "{}")) != nil {
		t.Fatal("a 200 is an error")
	}

	rec := httptest.NewRecorder()
	WriteError(rec, &resource.ConflictError{Key: resource.Key{Kind: "item", Name: "a"}, Expected: 3, Current: 4})
	err := ReadError(rec.Result())
	var e *Error
	if !errors.As(err, &e) || e.HTTPStatus != 412 || e.Code != resource.CodeConflict || e.Current != 4 {
		t.Fatalf("a written conflict read back = %+v", err)
	}
	var c *resource.ConflictError
	if !errors.Is(err, resource.ErrConflict) || !errors.As(err, &c) || c.Expected != 3 || c.Current != 4 || c.Key.Name != "a" {
		t.Fatalf("the conflict does not unwrap to its typed error: %v %+v", err, c)
	}
	if err.Error() != e.Message {
		t.Fatalf("message %q", err.Error())
	}

	rec = httptest.NewRecorder()
	WriteError(rec, Errorf(http.StatusPreconditionRequired, CodePreconditionRequired, "send one"))
	if err := ReadError(rec.Result()); !errors.As(err, &e) || e.Code != CodePreconditionRequired || e.HTTPStatus != 428 || errors.Is(err, resource.ErrConflict) {
		t.Fatalf("a route's own code = %v", err)
	}

	// Not a Status: the status names the code, the body is the message.
	if err := ReadError(answer(502, "text/html", " <h1>bad gateway</h1>\n")); !errors.As(err, &e) || e.Code != CodeInternal || e.Message != "<h1>bad gateway</h1>" {
		t.Fatalf("a proxy page = %+v", err)
	}
	if err := ReadError(answer(404, "text/plain", "")); !errors.As(err, &e) || e.Code != resource.CodeNotFound || e.Message != "Not Found" || !errors.Is(err, resource.ErrNotFound) {
		t.Fatalf("an empty 404 = %+v", err)
	}

	err = ReadError(answer(307, "text/plain", "", "Location", "https://elsewhere.example/x"))
	if !errors.As(err, &e) || e.HTTPStatus != 307 || !strings.Contains(e.Message, `"https://elsewhere.example/x"`) || strings.Contains(e.Message, "secret") {
		t.Fatalf("a redirect = %+v", err)
	}
}

// A server's own Error unwraps to nothing, so wrapping it changes no
// errors.Is the server already answers.
func TestServerErrorUnwrapsToNothing(t *testing.T) {
	if e := Errorf(412, resource.CodeConflict, "stale"); e.Unwrap() != nil || errors.Is(e, resource.ErrConflict) {
		t.Fatalf("%+v unwraps to %v", e, e.Unwrap())
	}
}
