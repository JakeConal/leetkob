// Package db holds the additive PostgreSQL initialization script for the
// PA#3 store. The postgres adapter applies InitSQL; callers do not need
// the process working directory to locate it.
package db

import _ "embed"

// InitSQL is the idempotent schema. Running it again creates nothing new
// and leaves existing rows in place.
//
//go:embed 001_init.sql
var InitSQL string
