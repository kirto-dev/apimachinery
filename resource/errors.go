// Copyright Kirto LLC 2026
// SPDX-License-Identifier: Apache-2.0

package resource

import (
	"errors"
	"fmt"
	"strings"
)

// Sentinels. Every typed error below matches one with errors.Is, and a
// product wraps any of them freely.
var (
	ErrNotFound    = errors.New("resource: not found")
	ErrConflict    = errors.New("resource: revision conflict")
	ErrSetConflict = errors.New("resource: name held by another set")
	ErrTooOld      = errors.New("resource: revision compacted")
	ErrInvalidName = errors.New("resource: invalid name")
	ErrInvalid     = errors.New("resource: refused by admission")
	ErrUnknownKind = errors.New("resource: unknown kind")
	ErrTooLarge    = errors.New("resource: too large")
	ErrQuota       = errors.New("resource: quota exceeded")
	ErrUnsupported = errors.New("resource: not supported by this backend")
)

// NotFoundError is a key or set with nothing stored under it.
type NotFoundError struct {
	Key Key
	// Set is the set name when the missing thing is a declared set.
	Set string
}

func (e *NotFoundError) Error() string {
	if e.Set != "" {
		return fmt.Sprintf("set %q not found", e.Set)
	}
	return e.Key.String() + " not found"
}

// Is matches [ErrNotFound].
func (e *NotFoundError) Is(target error) bool { return target == ErrNotFound }

// ConflictError is a precondition that did not hold.
type ConflictError struct {
	Key Key
	// Set is the set name for a set write's own revision.
	Set string
	// Expected is what the writer named. Current is what is stored: 0
	// when the object or set does not exist.
	Expected, Current uint64
	// Store is true when the store revision fence failed rather than the
	// object's; Current is then the store revision.
	Store bool
}

func (e *ConflictError) Error() string {
	switch {
	case e.Store:
		return fmt.Sprintf("store moved from revision %d to %d", e.Expected, e.Current)
	case e.Set != "" && e.Current == 0:
		return fmt.Sprintf("set %q does not exist (expected revision %d)", e.Set, e.Expected)
	case e.Set != "" && e.Expected == 0:
		return fmt.Sprintf("set %q already exists at revision %d", e.Set, e.Current)
	case e.Set != "":
		return fmt.Sprintf("set %q is at revision %d, not %d", e.Set, e.Current, e.Expected)
	case e.Current == 0:
		return fmt.Sprintf("%s does not exist (expected revision %d)", e.Key, e.Expected)
	case e.Expected == 0:
		return fmt.Sprintf("%s already exists at revision %d", e.Key, e.Current)
	default:
		return fmt.Sprintf("%s is at revision %d, not %d", e.Key, e.Current, e.Expected)
	}
}

// Is matches [ErrConflict].
func (e *ConflictError) Is(target error) bool { return target == ErrConflict }

// SetConflict is one name another set holds.
type SetConflict struct {
	Key Key `json:"key"`
	// Holder is the set that holds it.
	Holder Membership `json:"holder"`
	// Revision is the held object's revision, the one an adoption names.
	Revision uint64 `json:"revision"`
}

// SetConflictError lists every name a write could not take, so a writer
// that means to adopt learns all of them from one refusal.
type SetConflictError struct {
	Conflicts []SetConflict
}

func (e *SetConflictError) Error() string {
	parts := make([]string, len(e.Conflicts))
	for i, c := range e.Conflicts {
		parts[i] = fmt.Sprintf("%s is held by %s set %q at revision %d", c.Key, c.Holder.Mode, c.Holder.Name, c.Revision)
	}
	return strings.Join(parts, "; ")
}

// Is matches [ErrSetConflict].
func (e *SetConflictError) Is(target error) bool { return target == ErrSetConflict }

// TooOldError is a cursor the store can no longer serve from: below the
// compaction floor, or from another epoch. The reader lists again.
type TooOldError struct {
	Revision  uint64
	Compacted uint64
	// Epoch is the store's, when the cursor named another.
	Epoch string
}

func (e *TooOldError) Error() string {
	if e.Epoch != "" {
		return fmt.Sprintf("revision %d belongs to another history than epoch %q", e.Revision, e.Epoch)
	}
	return fmt.Sprintf("revision %d is compacted, the floor is %d", e.Revision, e.Compacted)
}

// Is matches [ErrTooOld].
func (e *TooOldError) Is(target error) bool { return target == ErrTooOld }

// InvalidNameError is a kind, namespace, name or set name the rules
// refuse.
type InvalidNameError struct {
	// Field is "kind", "namespace", "name" or "set".
	Field  string
	Value  string
	Reason string
	// Err is the kind's own ValidName error, when it refused.
	Err error
}

func (e *InvalidNameError) Error() string {
	return fmt.Sprintf("%s %q: %s", e.Field, e.Value, e.Reason)
}

// Is matches [ErrInvalidName].
func (e *InvalidNameError) Is(target error) bool { return target == ErrInvalidName }

// Unwrap is the kind's own error.
func (e *InvalidNameError) Unwrap() error { return e.Err }

// InvalidError is a write the server or a kind's validator refused.
type InvalidError struct {
	Key    Key
	Reason string
	// Err is the validator's error, kept so a product's own sentinel
	// matches through the wrap.
	Err error
}

func (e *InvalidError) Error() string {
	if e.Key == (Key{}) {
		return e.Reason
	}
	if e.Key.Name == "" {
		return fmt.Sprintf("%s: %s", e.Key.Kind, e.Reason)
	}
	return fmt.Sprintf("%s: %s", e.Key, e.Reason)
}

// Is matches [ErrInvalid].
func (e *InvalidError) Is(target error) bool { return target == ErrInvalid }

// Unwrap is the validator's error.
func (e *InvalidError) Unwrap() error { return e.Err }

// codeError is a sentinel with its own message, the form a sentinel-only
// code takes after crossing a node.
type codeError struct {
	sentinel error
	msg      string
}

func (e *codeError) Error() string        { return e.msg }
func (e *codeError) Is(target error) bool { return target == e.sentinel }

// Errorf wraps a sentinel with a message that does not repeat the
// sentinel's own text: errors.Is matches the sentinel, and Error is the
// message alone. It is the form a refusal with no typed error of its
// own takes, such as a quota or a size limit.
func Errorf(sentinel error, format string, args ...any) error {
	return &codeError{sentinel: sentinel, msg: fmt.Sprintf(format, args...)}
}

// Code names a typed error on the wire.
type Code string

const (
	// CodeNotFound: a key or set with nothing stored under it, and any 404
	// that names no code of its own, such as a path no route serves
	CodeNotFound Code = "not_found"
	// CodeConflict: a precondition that did not hold, the revision the
	// writer named against the one stored
	CodeConflict Code = "conflict"
	// CodeSetConflict: a name another set holds, listed with its holder and
	// revision for every name the write could not take
	CodeSetConflict Code = "set_conflict"
	// CodeTooOld: a cursor below the compaction floor or from another
	// epoch, so the reader lists again
	CodeTooOld Code = "too_old"
	// CodeInvalidName: a kind, namespace, name or set name the naming
	// rules or the kind's own name rule refuse
	CodeInvalidName Code = "invalid_name"
	// CodeInvalid: a request that parsed and was refused on what it says: a
	// document the kind's validator rejected, an object, set write or filter
	// the store refuses, or a precondition the route does not take, such as
	// If-Match sent beside If-None-Match
	CodeInvalid Code = "invalid"
	// CodeUnknownKind: a kind the store has no registration for
	CodeUnknownKind Code = "unknown_kind"
	// CodeTooLarge: a document past its kind's byte limit, an object naming
	// more blobs than the store allows, or a set write past its byte limit
	CodeTooLarge Code = "too_large"
	// CodeQuota: a write that would take the store past its object or set
	// limit
	CodeQuota Code = "quota"
	// CodeUnsupported: an operation the store's backend does not provide,
	// such as a set call on a backend that records no sets
	CodeUnsupported Code = "unsupported"
)

// Status is a typed error as data: what a raft apply result, a forward
// hop, a gRPC status detail or an HTTP body carries, so a refusal that
// crossed a node is the same error on the far side. Every field of every
// typed error has a place here.
type Status struct {
	Code      Code          `json:"code"`
	Message   string        `json:"message"`
	Key       *Key          `json:"key,omitempty"`
	Set       string        `json:"set,omitempty"`
	Expected  uint64        `json:"expected,omitempty"`
	Current   uint64        `json:"current,omitempty"`
	Store     bool          `json:"store,omitempty"`
	Revision  uint64        `json:"revision,omitempty"`
	Compacted uint64        `json:"compacted,omitempty"`
	Epoch     string        `json:"epoch,omitempty"`
	Field     string        `json:"field,omitempty"`
	Value     string        `json:"value,omitempty"`
	Reason    string        `json:"reason,omitempty"`
	Conflicts []SetConflict `json:"conflicts,omitempty"`
}

func keyPtr(k Key) *Key {
	if k == (Key{}) {
		return nil
	}
	return &k
}

func keyOf(k *Key) Key {
	if k == nil {
		return Key{}
	}
	return *k
}

// StatusOf converts a typed error, wrapped or not. ok is false for an
// error that is not one of this package's. Message is err's whole text,
// wrapping included.
func StatusOf(err error) (s Status, ok bool) {
	if err == nil {
		return Status{}, false
	}
	s.Message = err.Error()
	var (
		sc *SetConflictError
		c  *ConflictError
		nf *NotFoundError
		to *TooOldError
		in *InvalidNameError
		iv *InvalidError
	)
	switch {
	case errors.As(err, &sc):
		s.Code, s.Conflicts = CodeSetConflict, append([]SetConflict(nil), sc.Conflicts...)
	case errors.As(err, &c):
		s.Code, s.Key, s.Set, s.Expected, s.Current, s.Store = CodeConflict, keyPtr(c.Key), c.Set, c.Expected, c.Current, c.Store
	case errors.As(err, &nf):
		s.Code, s.Key, s.Set = CodeNotFound, keyPtr(nf.Key), nf.Set
	case errors.As(err, &to):
		s.Code, s.Revision, s.Compacted, s.Epoch = CodeTooOld, to.Revision, to.Compacted, to.Epoch
	case errors.As(err, &in):
		s.Code, s.Field, s.Value, s.Reason = CodeInvalidName, in.Field, in.Value, in.Reason
	case errors.As(err, &iv):
		s.Code, s.Key, s.Reason = CodeInvalid, keyPtr(iv.Key), iv.Reason
	default:
		for _, code := range sentinelOrder {
			if errors.Is(err, sentinels[code]) {
				s.Code = code
				return s, true
			}
		}
		return Status{}, false
	}
	return s, true
}

// sentinelOrder fixes the order StatusOf tests sentinels in, so an
// error that wraps two converts the same way every time.
var sentinelOrder = []Code{
	CodeSetConflict, CodeConflict, CodeNotFound, CodeTooOld, CodeInvalidName,
	CodeInvalid, CodeUnknownKind, CodeTooLarge, CodeQuota, CodeUnsupported,
}

var sentinels = map[Code]error{
	CodeNotFound:    ErrNotFound,
	CodeConflict:    ErrConflict,
	CodeSetConflict: ErrSetConflict,
	CodeTooOld:      ErrTooOld,
	CodeInvalidName: ErrInvalidName,
	CodeInvalid:     ErrInvalid,
	CodeUnknownKind: ErrUnknownKind,
	CodeTooLarge:    ErrTooLarge,
	CodeQuota:       ErrQuota,
	CodeUnsupported: ErrUnsupported,
}

// Err converts a status back to its typed error, with every field the
// status carries. A validator's own error does not survive the trip; its
// message does, as the Reason. A code this package does not know is a
// plain error with the message.
func (s Status) Err() error {
	switch s.Code {
	case CodeSetConflict:
		return &SetConflictError{Conflicts: append([]SetConflict(nil), s.Conflicts...)}
	case CodeConflict:
		return &ConflictError{Key: keyOf(s.Key), Set: s.Set, Expected: s.Expected, Current: s.Current, Store: s.Store}
	case CodeNotFound:
		return &NotFoundError{Key: keyOf(s.Key), Set: s.Set}
	case CodeTooOld:
		return &TooOldError{Revision: s.Revision, Compacted: s.Compacted, Epoch: s.Epoch}
	case CodeInvalidName:
		return &InvalidNameError{Field: s.Field, Value: s.Value, Reason: s.Reason}
	case CodeInvalid:
		return &InvalidError{Key: keyOf(s.Key), Reason: s.Reason}
	}
	if sentinel, ok := sentinels[s.Code]; ok {
		return &codeError{sentinel: sentinel, msg: s.Message}
	}
	return errors.New(s.Message)
}
