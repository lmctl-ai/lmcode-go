// Package stdlib re-exports the built-in catalogs for compatibility and
// explicit opt-out registration. The default Registry includes all catalogs.
// String functions operate on raw bytes: offsets and lengths are byte based,
// NUL bytes and invalid UTF-8 are preserved, and case conversion is ASCII only.
package stdlib
