// Package env reads Synthwerk environment variables (D-40).
//
//   - Service settings: SYNTHWERK_<SERVICE>_<NAME>, for example SYNTHWERK_PULSE_HTTP_ADDR.
//   - Shared platform settings: SYNTHWERK_PLATFORM_<NAME>, for example SYNTHWERK_PLATFORM_ENV.
//   - OpenTelemetry: the standard OTEL_* variables, no prefix. Package otel reads them.
package env

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Environments are the values of SYNTHWERK_PLATFORM_ENV.
var Environments = []string{"local", "ci", "dev", "stg", "prd"}

var serviceRe = regexp.MustCompile(`^[a-z][a-z0-9]*$`)

// Platform returns SYNTHWERK_PLATFORM_ENV. Empty means "local".
func Platform() (string, error) {
	v := os.Getenv("SYNTHWERK_PLATFORM_ENV")
	if v == "" {
		return "local", nil
	}
	for _, e := range Environments {
		if v == e {
			return v, nil
		}
	}
	return "", fmt.Errorf("env: SYNTHWERK_PLATFORM_ENV=%q is not one of %s", v, strings.Join(Environments, ", "))
}

// Key returns the variable name SYNTHWERK_<SERVICE>_<NAME>. service is the short
// service name in lowercase, for example "pulse"; name is the setting, for example "HTTP_ADDR".
func Key(service, name string) string {
	return "SYNTHWERK_" + strings.ToUpper(service) + "_" + strings.ToUpper(name)
}

// Get returns SYNTHWERK_<SERVICE>_<NAME>, or def when it is not set or empty.
func Get(service, name, def string) string {
	if v := os.Getenv(Key(service, name)); v != "" {
		return v
	}
	return def
}

// ValidService reports whether service is a valid short service name ("pulse", "llm").
func ValidService(service string) bool { return serviceRe.MatchString(service) }
