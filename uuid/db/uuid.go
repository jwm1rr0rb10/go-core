// Package db provides UUID constructors intended for database keys.
//
// It is a thin, explicit-intent layer over the root uuid package: values use
// crypto/rand and NewV7 is strictly monotonic, which keeps B-tree indexes
// append-only. UUID is an alias of uuid.UUID, so values from db, network and
// uuid are interchangeable and implement sql.Scanner / driver.Valuer.
package db

import "github.com/jwm1rr0rb10/go-core/uuid"

// UUID is an alias of uuid.UUID.
type UUID = uuid.UUID

// NewV4 returns a cryptographically random version 4 UUID.
//
// The error is always nil (crypto/rand cannot fail since Go 1.24); it is kept
// for API compatibility.
func NewV4() (UUID, error) { return uuid.NewV4(), nil }

// NewV7 returns a time-ordered, strictly monotonic version 7 UUID with
// cryptographically random bits. Prefer it for primary keys.
//
// The error is always nil; it is kept for API compatibility.
func NewV7() (UUID, error) { return uuid.NewV7(), nil }
