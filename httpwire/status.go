// Copyright Kirto LLC 2026
// SPDX-License-Identifier: Apache-2.0

package httpwire

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"kirto.dev/apimachinery/resource"
)

// Status is every error body: [resource.Status], whose JSON it keeps
// field for field, plus the request-validation detail in Causes.
type Status struct {
	resource.Status
	Causes []Cause `json:"causes,omitempty"`
}

// Cause is one refused part of a request.
type Cause struct {
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

// The codes an HTTP route answers beside the store's own.
const (
	// CodeBadRequest: a request that did not parse, such as a malformed
	// If-Match or watch query, and any 4xx no other code claims
	CodeBadRequest resource.Code = "bad_request"
	// CodeAlreadyExists: a name something else already holds, refused by a
	// handler's own check rather than by a precondition, and any 409 that
	// names no code of its own
	CodeAlreadyExists resource.Code = "already_exists"
	// CodeUnauthorized: a request whose caller could not be authenticated, and
	// any 401 that names no code of its own
	CodeUnauthorized resource.Code = "unauthorized"
	// CodeForbidden: an authenticated caller the route's authorization
	// refused, and any 403 that names no code of its own
	CodeForbidden resource.Code = "forbidden"
	// CodeMethodNotAllowed: a method the route does not take, answered with
	// the Allow header naming the ones it does
	CodeMethodNotAllowed resource.Code = "method_not_allowed"
	// CodePreconditionRequired: a write that can replace or remove state
	// sent with neither If-Match nor If-None-Match, or a delete without
	// If-Match
	CodePreconditionRequired resource.Code = "precondition_required"
	// CodeTooManyRequests: a watch past the number of open streams one
	// caller may hold, and any 429 that names no code of its own
	CodeTooManyRequests resource.Code = "too_many_requests"
	// CodeInternal: an error that is neither a route's nor the store's,
	// answered as 500, and any 5xx no other code claims
	CodeInternal resource.Code = "internal"
	// CodeUnavailable: a server shutting down, and any 503 that names no code
	// of its own
	CodeUnavailable resource.Code = "unavailable"
)

// Error is a Status with the HTTP status it answers. The HTTP status is
// not part of the body, so a framework that reflects the type for its
// schema and marshals it as the body gets exactly the Status fields.
type Error struct {
	Status
	HTTPStatus int `json:"-"`

	// typed is the store error the status decodes to, set by
	// [ReadError] so a client's errors.Is and errors.As reach it. A
	// server's own Error leaves it nil and unwraps to nothing.
	typed error
}

func (e *Error) Error() string { return e.Message }

// Unwrap is the typed error a decoded status carries: errors.Is(err,
// resource.ErrConflict) holds for a 412 conflict a client read, and
// errors.As reaches the *resource.ConflictError with its revisions.
func (e *Error) Unwrap() error { return e.typed }

// GetStatus is the HTTP status.
func (e *Error) GetStatus() int { return e.HTTPStatus }

// Errorf is an error answering status with code; code "" is
// [GenericCode] of status.
func Errorf(status int, code resource.Code, format string, args ...any) *Error {
	if code == "" {
		code = GenericCode(status)
	}
	return &Error{Status: Status{Status: resource.Status{Code: code, Message: fmt.Sprintf(format, args...)}}, HTTPStatus: status}
}

// FromError is err as an HTTP error. An *Error, wrapped or not, passes
// through; a store error keeps every field [resource.StatusOf] gives it
// and answers [HTTPStatus] of its code; anything else is 500 internal.
// FromError of nil is nil.
func FromError(err error) *Error {
	if err == nil {
		return nil
	}
	if e, ok := errors.AsType[*Error](err); ok {
		return e
	}
	if s, ok := resource.StatusOf(err); ok {
		return &Error{Status: Status{Status: s}, HTTPStatus: HTTPStatus(s.Code)}
	}
	return Errorf(http.StatusInternalServerError, CodeInternal, "%s", err.Error())
}

var httpStatus = map[resource.Code]int{
	resource.CodeConflict:    http.StatusPreconditionFailed,
	CodePreconditionRequired: http.StatusPreconditionRequired,
	resource.CodeSetConflict: http.StatusConflict,
	CodeAlreadyExists:        http.StatusConflict,
	resource.CodeTooOld:      http.StatusGone,
	resource.CodeNotFound:    http.StatusNotFound,
	resource.CodeUnknownKind: http.StatusNotFound,
	resource.CodeInvalid:     http.StatusUnprocessableEntity,
	CodeBadRequest:           http.StatusBadRequest,
	resource.CodeInvalidName: http.StatusBadRequest,
	resource.CodeTooLarge:    http.StatusRequestEntityTooLarge,
	resource.CodeQuota:       http.StatusConflict,
	CodeUnauthorized:         http.StatusUnauthorized,
	CodeForbidden:            http.StatusForbidden,
	CodeMethodNotAllowed:     http.StatusMethodNotAllowed,
	CodeTooManyRequests:      http.StatusTooManyRequests,
	CodeInternal:             http.StatusInternalServerError,
	resource.CodeUnsupported: http.StatusNotImplemented,
	CodeUnavailable:          http.StatusServiceUnavailable,
}

// HTTPStatus is the status a code answers: a stale precondition 412, a
// missing one 428, a name another set holds or another object already
// takes 409, a cursor too old 410,
// no such object or kind 404, a refused value 422, a request that did
// not parse or a name that cannot exist 400. A code outside the table
// is 500.
func HTTPStatus(c resource.Code) int {
	if s, ok := httpStatus[c]; ok {
		return s
	}
	return http.StatusInternalServerError
}

var genericCode = map[int]resource.Code{
	http.StatusBadRequest:            CodeBadRequest,
	http.StatusUnauthorized:          CodeUnauthorized,
	http.StatusForbidden:             CodeForbidden,
	http.StatusNotFound:              resource.CodeNotFound,
	http.StatusMethodNotAllowed:      CodeMethodNotAllowed,
	http.StatusConflict:              CodeAlreadyExists,
	http.StatusGone:                  resource.CodeTooOld,
	http.StatusPreconditionFailed:    resource.CodeConflict,
	http.StatusRequestEntityTooLarge: resource.CodeTooLarge,
	http.StatusUnprocessableEntity:   resource.CodeInvalid,
	http.StatusPreconditionRequired:  CodePreconditionRequired,
	http.StatusTooManyRequests:       CodeTooManyRequests,
	http.StatusInternalServerError:   CodeInternal,
	http.StatusNotImplemented:        resource.CodeUnsupported,
	http.StatusServiceUnavailable:    CodeUnavailable,
}

// GenericCode is the code for an error that has only its HTTP status: a
// handler's own refusal. 409 is already_exists, never conflict, which
// means a failed precondition (412) everywhere in the table; a 409 that
// is neither names its own code. Another 4xx is bad_request, another 5xx
// internal.
func GenericCode(status int) resource.Code {
	if c, ok := genericCode[status]; ok {
		return c
	}
	if status >= 500 {
		return CodeInternal
	}
	return CodeBadRequest
}

// WriteError writes [FromError] of err as the response.
func WriteError(w http.ResponseWriter, err error) {
	e := FromError(err)
	if e == nil {
		e = Errorf(http.StatusInternalServerError, CodeInternal, "no error")
	}
	WriteJSON(w, e.HTTPStatus, e.Status)
}

// WriteJSON writes v as a JSON response with status.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// MaxErrorBytes bounds the error body [ReadError] reads.
const MaxErrorBytes = 1 << 20

// ReadError is a client's error for an answer of 300 or more: the body's
// Status with resp's HTTP status, unwrapping to the typed error its code
// names ([resource.Status.Err]). A body that is not a Status (a proxy's
// page, a route the server does not serve) is the status's generic code
// ([GenericCode]) with the body's text as the message. A redirect is an
// error naming its Location, since a client that follows one replays its
// credentials and its write to wherever it points. ReadError reads the
// body and leaves closing it to the caller; it is nil below 300.
func ReadError(resp *http.Response) error {
	if resp.StatusCode < 300 {
		return nil
	}
	e := &Error{HTTPStatus: resp.StatusCode}
	if resp.StatusCode < 400 {
		e.Code = GenericCode(resp.StatusCode)
		e.Message = fmt.Sprintf("%s %s answered %d, a redirect to %q, which is not followed",
			resp.Request.Method, resp.Request.URL.Redacted(), resp.StatusCode, resp.Header.Get("Location"))
		return e
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, MaxErrorBytes))
	if err != nil {
		return fmt.Errorf("reading a %d answer: %w", resp.StatusCode, err)
	}
	if json.Unmarshal(raw, &e.Status) != nil || e.Code == "" {
		e.Status = Status{Status: resource.Status{Code: GenericCode(resp.StatusCode), Message: strings.TrimSpace(string(raw))}}
		if e.Message == "" {
			e.Message = http.StatusText(resp.StatusCode)
		}
	}
	return ErrorFrom(e.Status, resp.StatusCode)
}

// ErrorFrom is a client's error for s, a Status read off the wire with
// the HTTP status it answered, or a watch stream's ERROR line with
// [HTTPStatus] of its code. It unwraps to the typed error s's code names.
func ErrorFrom(s Status, httpStatus int) *Error {
	return &Error{Status: s, HTTPStatus: httpStatus, typed: s.Status.Err()}
}
