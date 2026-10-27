// Copyright Kirto LLC 2026
// SPDX-License-Identifier: Apache-2.0

package resource

import (
	"fmt"
	"regexp"
	"unicode"
	"unicode/utf8"
)

// MaxNameBytes bounds a name.
const MaxNameBytes = 253

// The name rules every server applies. A kind's own name rule may narrow
// the rule for names and never widen it.
var (
	// kindRE admits the underscores of kinds that came from document
	// keys, such as a key named after two words.
	kindRE = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,62}$`)
	// labelRE is a DNS label of up to 64 characters. A namespace and a
	// set name end up in a URL path segment, a Cedar entity id and a
	// Kubernetes object name, and this is the rule all three accept.
	labelRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,62}[a-z0-9])?$`)
)

// ValidKind is the rule for a kind: 1-63 characters of lowercase letters,
// digits, dashes and underscores, starting with a letter.
func ValidKind(s string) error {
	if !kindRE.MatchString(s) {
		return &InvalidNameError{Field: "kind", Value: s, Reason: "a kind is 1-63 lowercase letters, digits, dashes or underscores, starting with a letter"}
	}
	return nil
}

// ValidNamespace is the rule for a namespace: a DNS label of 1-64
// characters.
func ValidNamespace(s string) error {
	if !labelRE.MatchString(s) {
		return &InvalidNameError{Field: "namespace", Value: s, Reason: "a namespace is a DNS label of 1-64 lowercase letters, digits and dashes"}
	}
	return nil
}

// ValidSetName is the rule for a declared set's name, the namespace rule.
func ValidSetName(s string) error {
	if !labelRE.MatchString(s) {
		return &InvalidNameError{Field: "set", Value: s, Reason: "a set name is a DNS label of 1-64 lowercase letters, digits and dashes"}
	}
	return nil
}

// ValidName is the rule for a name: 1-253 bytes of UTF-8 with no control
// characters and no leading or trailing space. Slashes and "@" are
// allowed, so a product can name versions and paths. NUL is a control
// character, and its absence is what lets a backend build keys
// separated by NUL.
func ValidName(s string) error {
	bad := func(reason string) error { return &InvalidNameError{Field: "name", Value: s, Reason: reason} }
	switch {
	case s == "":
		return bad("a name is required")
	case len(s) > MaxNameBytes:
		return bad(fmt.Sprintf("a name is at most %d bytes", MaxNameBytes))
	case !utf8.ValidString(s):
		return bad("a name is valid UTF-8")
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return bad("a name has no control characters")
		}
	}
	first, _ := utf8.DecodeRuneInString(s)
	last, _ := utf8.DecodeLastRuneInString(s)
	if unicode.IsSpace(first) || unicode.IsSpace(last) {
		return bad("a name has no leading or trailing space")
	}
	return nil
}
