package google_uuid

import "github.com/google/uuid"

// GoogleUUIDGenerator generates RFC 9562 version 4 UUID strings.
type GoogleUUIDGenerator struct{}

// NewGoogleUUIDGenerator returns a version 4 UUID generator.
func NewGoogleUUIDGenerator() *GoogleUUIDGenerator { return &GoogleUUIDGenerator{} }

// GenerateID returns a random version 4 UUID in canonical form.
func (*GoogleUUIDGenerator) GenerateID() string { return uuid.NewString() }

// GoogleUUIDv7Generator generates time-ordered version 7 UUID strings.
type GoogleUUIDv7Generator struct{}

// NewGoogleUUIDv7Generator returns a version 7 UUID generator.
func NewGoogleUUIDv7Generator() *GoogleUUIDv7Generator { return &GoogleUUIDv7Generator{} }

// GenerateID returns a version 7 UUID in canonical form. google/uuid
// guarantees monotonic ordering within the process.
func (*GoogleUUIDv7Generator) GenerateID() string {
	return uuid.Must(uuid.NewV7()).String()
}
