package domain

import "regexp"

var identifierPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{2,63}$`)

func ValidateIdentifier(field, value string) error {
	if identifierPattern.MatchString(value) {
		return nil
	}
	return NewError(
		"invalid_identifier",
		400,
		"%s must start with a lowercase letter and use 3-64 lowercase letters, digits, underscores, or hyphens",
		field,
	)
}
