package hardrule

// Decide applies one constraint to a parsed analyzer result.
// source is read only to copy evidence lines; banned names are taken from
// the constraint and matched against result references.
func Decide(constraintText, source string, result Result) Decision {
	return Decision{}
}

// Evaluate parses a versioned analyzer document and decides the constraint.
// Malformed or unsupported analysis becomes unsure for a ban list and is
// never reported as a violation.
func Evaluate(constraintText, source string, raw []byte) Decision {
	return Decision{}
}
