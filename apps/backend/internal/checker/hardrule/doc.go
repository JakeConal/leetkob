// Package hardrule decides ban-list constraints from a bounded libclang
// result. The analyzer is a separate native program; this package never
// uses CGO and never searches submission text for a banned name.
package hardrule
