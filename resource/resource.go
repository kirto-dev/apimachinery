// Copyright Kirto LLC 2026
// SPDX-License-Identifier: Apache-2.0

package resource

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"
)

// Key addresses one object. Namespace is empty for a cluster-scoped kind
// and required for a namespaced one.
type Key struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
}

// String renders the key for messages: "kind name" or
// "kind namespace/name".
func (k Key) String() string {
	if k.Namespace == "" {
		return fmt.Sprintf("%s %q", k.Kind, k.Name)
	}
	return fmt.Sprintf("%s %s/%q", k.Kind, k.Namespace, k.Name)
}

// Compare orders keys by kind, namespace, then name. It is the order a
// list, a set's members and the events of one revision come in.
func (k Key) Compare(o Key) int {
	return cmp.Or(strings.Compare(k.Kind, o.Kind), strings.Compare(k.Namespace, o.Namespace), strings.Compare(k.Name, o.Name))
}

// SetMode is how an object came to belong to its set.
type SetMode int

const (
	// SetNone is an object on a backend that records no sets.
	SetNone SetMode = iota
	// SetImperative is an object created singly, through the object
	// path.
	SetImperative
	// SetDeclared is a member materialized from a declared set write. It
	// is read-only through the object path.
	SetDeclared
)

// String is the mode as a policy attribute reads it: "imperative",
// "declared" or empty.
func (m SetMode) String() string {
	switch m {
	case SetImperative:
		return "imperative"
	case SetDeclared:
		return "declared"
	default:
		return ""
	}
}

// Membership is the set label the server records on an object. Name is
// the declared set's name, or the store's imperative set name. The
// caller never supplies it: the server assigns it from the path the
// write took.
type Membership struct {
	Name string  `json:"name,omitempty"`
	Mode SetMode `json:"mode,omitempty"`
}

// PrincipalKind is how a caller was identified. The set is closed: an
// audit reader filters on it and a policy branches on it, and neither
// can be written against a vocabulary that grows a value per call site.
type PrincipalKind string

const (
	// PrincipalHuman is a person, signed in at an identity provider. An
	// OIDC token from an issuer that is not marked as a workload platform
	// lands here.
	PrincipalHuman PrincipalKind = "human"
	// PrincipalWorkload is a program acting under its own identity: a
	// Kubernetes service account token, a CI job's OIDC token, a SPIFFE
	// X.509 or JWT SVID.
	PrincipalWorkload PrincipalKind = "workload"
	// PrincipalNode is a member of the product's own cluster, identified
	// by the certificate it holds on the cluster link.
	PrincipalNode PrincipalKind = "node"
	// PrincipalKey is a caller presenting a credential the product
	// minted itself, named by the credential. What the key grants is the
	// product's business.
	PrincipalKey PrincipalKind = "key"
	// PrincipalLocal is a process on the other end of a unix socket,
	// named by the uid the kernel recorded at connect time. It is
	// attributable: the operating system, not the caller, says who it
	// is.
	PrincipalLocal PrincipalKind = "local"
	// PrincipalInsecure is a caller on a listener configured to
	// authenticate nobody, which trusts whoever reaches it. Development
	// listeners and issuerless loopback doors produce it. It is a real
	// kind rather than an absent one because "anybody who can reach this
	// socket" is the honest name for that principal, and a trail that
	// recorded it as anonymous would hide that it was granted
	// everything.
	PrincipalInsecure PrincipalKind = "insecure"
	// PrincipalAnonymous is a caller who presented no credential on a
	// door that asks for one. It holds no roles.
	PrincipalAnonymous PrincipalKind = "anonymous"
	// PrincipalSystem is the product acting on its own behalf: a node
	// applying its file at start, a scheduled task, a timer.
	PrincipalSystem PrincipalKind = "system"
)

// PrincipalKinds lists every kind, in declaration order.
func PrincipalKinds() []PrincipalKind {
	return []PrincipalKind{PrincipalHuman, PrincipalWorkload, PrincipalNode, PrincipalKey, PrincipalLocal, PrincipalInsecure, PrincipalAnonymous, PrincipalSystem}
}

// Valid reports whether k is one of [PrincipalKinds].
func (k PrincipalKind) Valid() bool { return slices.Contains(PrincipalKinds(), k) }

// Writer is who a write is attributed to and when they proposed it. At
// is the proposer's clock, the only clock a replicated server stamps
// timestamps from, so every replica stamps the same time.
type Writer struct {
	// Principal is the principal's qualified id: the Cedar entity id.
	Principal string        `json:"principal,omitempty"`
	Kind      PrincipalKind `json:"kind,omitempty"`
	Name      string        `json:"name,omitempty"`
	At        time.Time     `json:"at,omitzero"`
}

// Precondition is what a write requires of the store where it applies.
// For a replicated backend that is in the apply path, so every replica
// reaches the same answer.
type Precondition struct {
	// Revision is the object's (or the set's) current revision. 0 means
	// it must not exist, which is how create is spelled. A delete must
	// name a non-zero revision.
	Revision uint64 `json:"revision,omitempty"`
	// StoreRevision, when non-zero, requires the store revision to equal
	// it: any write anywhere since refuses this one with a store
	// conflict. It fences a write validated against a whole listing, such
	// as a composition of every set. 0 is no fence, and no head is 0, so
	// a revision read from a head or a list always fences.
	StoreRevision uint64 `json:"storeRevision,omitempty"`
}

// EventType is what a watch event reports.
type EventType int

const (
	// Added is an object the watcher has not seen in its current life:
	// its creation is after the point the watcher's view was last exact.
	Added EventType = iota + 1
	// Modified is a later revision of an object the watcher has seen.
	Modified
	// Deleted carries the tombstone.
	Deleted
	// Bookmark carries no object. Its revision is a point at which the
	// events delivered so far equal a list at that revision.
	Bookmark
)

// String names the event type.
func (t EventType) String() string {
	switch t {
	case Added:
		return "added"
	case Modified:
		return "modified"
	case Deleted:
		return "deleted"
	case Bookmark:
		return "bookmark"
	default:
		return fmt.Sprintf("EventType(%d)", int(t))
	}
}
