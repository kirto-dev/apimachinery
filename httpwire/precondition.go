// Copyright Kirto LLC 2026
// SPDX-License-Identifier: Apache-2.0

package httpwire

import (
	"net/http"
	"strconv"
	"strings"

	"kirto.dev/apimachinery/resource"
)

// ParsePrecondition reads a write's conditional headers:
//
//   - If-None-Match: * is create, Precondition{Revision: 0};
//   - If-Match: "N" (or W/"N", or a list naming one N), N > 0, is
//     Precondition{Revision: N};
//   - neither, on a route that requires one, is 428
//     precondition_required, and on one that does not, the zero
//     Precondition, left to the route's own create or final-state rule;
//   - both, If-Match: *, If-Match: "0", a list naming two revisions, or
//     an If-None-Match other than * is 422 invalid;
//   - an If-Match that is not a quoted number, or a list of them, is 400
//     bad_request.
//
// A write requires one whenever it can replace or remove state a caller
// may have read: an unconditional replace is the lost update the store's
// compare-and-swap exists to refuse.
func ParsePrecondition(ifMatch, ifNoneMatch string, required bool) (resource.Precondition, error) {
	ifMatch, ifNoneMatch = strings.TrimSpace(ifMatch), strings.TrimSpace(ifNoneMatch)
	switch {
	case ifMatch == "" && ifNoneMatch == "":
		if required {
			return resource.Precondition{}, Errorf(http.StatusPreconditionRequired, CodePreconditionRequired,
				`a precondition is required: If-None-Match: * to create, or If-Match: "<resourceVersion>" to change what you read`)
		}
		return resource.Precondition{}, nil
	case ifMatch != "" && ifNoneMatch != "":
		return resource.Precondition{}, invalid("If-Match and If-None-Match say different things; send one")
	case ifNoneMatch != "":
		if ifNoneMatch != "*" {
			return resource.Precondition{}, invalid("If-None-Match must be *, meaning the object must not exist yet")
		}
		return resource.Precondition{}, nil
	case ifMatch == "*":
		return resource.Precondition{}, invalid(`If-Match: * is not supported; send the resourceVersion you read, so a write over somebody else's is refused rather than applied`)
	}
	var rev uint64
	for tag := range strings.SplitSeq(ifMatch, ",") {
		n, ok := revisionTag(strings.TrimSpace(tag))
		if !ok {
			return resource.Precondition{}, Errorf(http.StatusBadRequest, CodeBadRequest,
				`If-Match %s is not a resourceVersion; send the ETag the object was read with`, ifMatch)
		}
		if n == 0 {
			return resource.Precondition{}, invalid(`If-Match: "0" is no revision at all; to create, send If-None-Match: *`)
		}
		if rev != 0 && n != rev {
			return resource.Precondition{}, invalid("If-Match names more than one revision; send the one you read")
		}
		rev = n
	}
	return resource.Precondition{Revision: rev}, nil
}

// revisionTag reads one entity tag, "N" or W/"N".
func revisionTag(tag string) (uint64, bool) {
	tag = strings.TrimPrefix(tag, "W/")
	if len(tag) < 3 || tag[0] != '"' || tag[len(tag)-1] != '"' {
		return 0, false
	}
	digits := tag[1 : len(tag)-1]
	if digits[0] < '0' || digits[0] > '9' {
		return 0, false
	}
	n, err := strconv.ParseUint(digits, 10, 64)
	return n, err == nil
}

func invalid(msg string) *Error {
	return Errorf(http.StatusUnprocessableEntity, resource.CodeInvalid, "%s", msg)
}

// Precondition is [ParsePrecondition] of r's headers, each header's
// lines joined as one list.
func Precondition(r *http.Request, required bool) (resource.Precondition, error) {
	return ParsePrecondition(strings.Join(r.Header.Values("If-Match"), ","), strings.Join(r.Header.Values("If-None-Match"), ","), required)
}

// ETag is a revision as a strong entity tag.
func ETag(rev uint64) string { return `"` + strconv.FormatUint(rev, 10) + `"` }

// SetETag sets the ETag header of a response that carries one object.
func SetETag(w http.ResponseWriter, rev uint64) { w.Header().Set("ETag", ETag(rev)) }

// SetPrecondition sets a write's conditional header from pre, the form
// [ParsePrecondition] reads back: Revision 0 is If-None-Match: *, any
// other revision If-Match with its [ETag]. pre.StoreRevision has no
// header and is not sent.
func SetPrecondition(h http.Header, pre resource.Precondition) {
	if pre.Revision == 0 {
		h.Set("If-None-Match", "*")
		return
	}
	h.Set("If-Match", ETag(pre.Revision))
}
