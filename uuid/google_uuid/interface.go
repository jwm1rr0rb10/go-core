// Package google_uuid provides string ID generators behind a common
// IDGenerator interface: UUIDv4/UUIDv7 backed by github.com/google/uuid and
// ULIDs backed by github.com/oklog/ulid/v2.
//
// The directory name contains an underscore for historical reasons; import it
// with an alias:
//
//	import idgen "github.com/jwm1rr0rb10/go-core/uuid/google_uuid"
//
// All generators are safe for concurrent use.
package google_uuid

// IDGenerator produces unique string identifiers. Implementations in this
// package are safe for concurrent use and never fail.
type IDGenerator interface {
	GenerateID() string
}

// Compile-time interface checks.
var (
	_ IDGenerator = (*GoogleUUIDGenerator)(nil)
	_ IDGenerator = (*GoogleUUIDv7Generator)(nil)
	_ IDGenerator = (*ULIDGenerator)(nil)
)
