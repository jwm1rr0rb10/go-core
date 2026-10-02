// Package pointer provides small generic helpers for optional values
// represented as pointers: creating them from literals, reading them with
// fallbacks, comparing and copying them.
//
// Since Go 1.26 the builtin new accepts an expression (new(42)), which
// covers what ToPointer does; the rest of the package has no builtin
// equivalent.
package pointer
