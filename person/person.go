// Package person provides read-only intelligence views over one enrolled
// person: their identity chain (identity_document/identity_register), the
// biometric clusters their enrolled features have resolved into, and the
// biometric cases linked to them through those clusters.
//
// Every view here is a Postgres-only projection over person / identity_* /
// biometricfeature / cluster_* / case_* (see MODEL.md) -- there is no Neo4j
// dependency. A person is never linked to a cluster or a case by raw
// co-occurrence: the link always runs through cluster_members, which in turn
// only grows from a CONFIRMED biometric_decisions chain (see
// cluster.DeriveEdgeStatus and cluster.Identify). This package writes
// nothing; it reads what cluster.Run/cluster.Identify have already derived.
package person

import "errors"

// ErrNotFound is returned when the given person_id has no person row.
var ErrNotFound = errors.New("person: person not found")
