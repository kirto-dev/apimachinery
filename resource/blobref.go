// Copyright Kirto LLC 2026
// SPDX-License-Identifier: Apache-2.0

package resource

import (
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidBlobRef matches every error [BlobRef.Validate] returns.
var ErrInvalidBlobRef = errors.New("blob: invalid ref")

// BlobRef names a payload held outside the store by its content hash.
// The store keeps the reference and never the bytes. A structured field
// rather than a convention inside an object's bytes lets a blob store
// find every referenced digest without parsing a product's bytes.
type BlobRef struct {
	// Digest is "sha256:" and 64 lowercase hex characters.
	Digest string `json:"digest"`
	// Size is the payload's length in bytes.
	Size int64 `json:"size"`
	// MediaType is the payload's type as the caller names it.
	MediaType string `json:"mediaType,omitempty"`
}

// Validate refuses a malformed digest or a negative size. The error
// matches [ErrInvalidBlobRef].
func (r BlobRef) Validate() error {
	if digestHex(r.Digest) == "" {
		return refError(fmt.Sprintf("digest %q is not sha256: and 64 lowercase hex characters", r.Digest))
	}
	if r.Size < 0 {
		return refError(fmt.Sprintf("size %d is negative", r.Size))
	}
	return nil
}

// Hex is the 64 hex characters of the digest, or "" when Digest is
// malformed.
func (r BlobRef) Hex() string { return digestHex(r.Digest) }

func digestHex(digest string) string {
	h, ok := strings.CutPrefix(digest, "sha256:")
	if !ok || len(h) != 64 {
		return ""
	}
	for i := 0; i < len(h); i++ {
		if c := h[i]; (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return ""
		}
	}
	return h
}

// refError keeps Validate's message free of a package prefix, since a
// caller such as an admission front wraps it in its own words.
type refError string

func (e refError) Error() string        { return string(e) }
func (e refError) Is(target error) bool { return target == ErrInvalidBlobRef }
