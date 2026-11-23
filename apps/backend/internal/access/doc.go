// Package access is the PA#3 identity boundary: synthetic assessment
// accounts, hashed opaque sessions, and current role and class checks.
// It does not import the database driver or net/http. Callers pass a
// Repository and apply the session cookie themselves.
package access
