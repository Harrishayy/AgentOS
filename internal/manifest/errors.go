package manifest

import (
	"fmt"
	"sort"
	"strings"
)

// Code is a stable identifier for a manifest validation failure. It appears in
// JSON error envelopes as `error.code` and in the catalogue documentation.
type Code string

const (
	CodeYAMLParse           Code = "yaml_parse"
	CodeUnknownField        Code = "unknown_field"
	CodeWrongKind           Code = "wrong_kind"
	CodeMissingRequired     Code = "missing_required"
	CodeInvalidName         Code = "invalid_name"
	CodeEmptyCommand        Code = "empty_command"
	CodeNonAbsolutePath     Code = "non_absolute_path"
	CodeInvalidHostPattern  Code = "invalid_host_pattern"
	CodeInvalidPathPattern  Code = "invalid_path_pattern"
	CodeUnsupportedFieldVal Code = "unsupported_field_value"
	CodeBadDuration         Code = "bad_duration"
	CodeUnsetEnvVar         Code = "unset_env_var"
	CodeBadUser             Code = "bad_user"
	CodeBadStdin            Code = "bad_stdin"
	CodeDuplicateKey        Code = "duplicate_key"
	CodeBadEnvKey           Code = "bad_env_key"
)

// Error is a single manifest validation failure with line/column precision.
//
// Multiple validation failures are bundled into MultiError, but the most common
// case (single failure) renders identically whether wrapped or not.
type Error struct {
	Code       Code   // catalog code
	Message    string // fully-rendered human message (without "Error: " prefix)
	Field      string // dotted path within the manifest (e.g. "allowed_paths[0]")
	Path       string // manifest filename, as given on the CLI
	Line       int    // 1-based line number from yaml.Node
	Column     int    // 1-based column number from yaml.Node
	Suggestion string // optional did-you-mean suggestion (unknown_field only)
}

// Error implements the error interface. Format:
//
//	<file>:<line>:<col>: <message>
//
// If Path/Line/Column are zero (no fixture file context, e.g. inline test data),
// the prefix collapses gracefully.
func (e *Error) Error() string {
	if e.Line == 0 && e.Path == "" {
		return e.Message
	}
	prefix := e.Path
	if e.Line > 0 {
		if prefix == "" {
			prefix = "<input>"
		}
		prefix = fmt.Sprintf("%s:%d:%d", prefix, e.Line, e.Column)
	}
	return prefix + ": " + e.Message
}

// MultiError carries more than one Error. The exit-code mapper treats it the
// same as a single Error (ExitManifestInvalid).
type MultiError struct {
	Errors []*Error
}

func (m *MultiError) Error() string {
	if len(m.Errors) == 0 {
		return "manifest invalid"
	}
	if len(m.Errors) == 1 {
		return m.Errors[0].Error()
	}
	parts := make([]string, len(m.Errors))
	for i, e := range m.Errors {
		parts[i] = e.Error()
	}
	return strings.Join(parts, "\n")
}

// Is satisfies errors.Is so callers can match `errors.Is(err, &Error{})` style
// checks. We treat MultiError as matching any single Error.
func (m *MultiError) Is(target error) bool {
	_, ok := target.(*Error)
	return ok
}

// Unwrap returns the first underlying Error so errors.As will pick it up.
func (m *MultiError) Unwrap() error {
	if len(m.Errors) == 0 {
		return nil
	}
	return m.Errors[0]
}

// errBuilder accumulates errors during a parse/validate pass.
type errBuilder struct {
	path string
	errs []*Error
}

func newErrBuilder(path string) *errBuilder { return &errBuilder{path: path} }

func (b *errBuilder) add(e *Error) {
	if e.Path == "" {
		e.Path = b.path
	}
	b.errs = append(b.errs, e)
}

// addf is a shorthand for adding an error with formatted message.
func (b *errBuilder) addf(code Code, line, col int, field, format string, args ...any) {
	b.add(&Error{
		Code:    code,
		Message: fmt.Sprintf(format, args...),
		Field:   field,
		Line:    line,
		Column:  col,
	})
}

// result returns nil if no errors were collected, the single Error if one was,
// or a MultiError otherwise.
func (b *errBuilder) result() error {
	if len(b.errs) == 0 {
		return nil
	}
	if len(b.errs) == 1 {
		return b.errs[0]
	}
	// Sort by line/col so output is stable.
	sort.SliceStable(b.errs, func(i, j int) bool {
		if b.errs[i].Line != b.errs[j].Line {
			return b.errs[i].Line < b.errs[j].Line
		}
		return b.errs[i].Column < b.errs[j].Column
	})
	return &MultiError{Errors: b.errs}
}

// formatUnknownFieldNoSuggestion renders the catalogue message for an unknown
// field with no Levenshtein candidate.
func formatUnknownFieldNoSuggestion(field string) string {
	return fmt.Sprintf(
		"unknown field %q; valid keys: %s",
		field,
		strings.Join(KnownTopLevelKeys, ", "),
	)
}

// formatUnknownFieldWithSuggestion renders the catalogue message with a
// did-you-mean suggestion. If multiple suggestions, list all separated by " or ".
func formatUnknownFieldWithSuggestion(field string, suggestions []string) string {
	switch len(suggestions) {
	case 0:
		return formatUnknownFieldNoSuggestion(field)
	case 1:
		return fmt.Sprintf("unknown field %q (did you mean %q?)", field, suggestions[0])
	default:
		return fmt.Sprintf("unknown field %q (did you mean %s?)", field, joinQuoted(suggestions, " or "))
	}
}

func joinQuoted(items []string, sep string) string {
	q := make([]string, len(items))
	for i, s := range items {
		q[i] = fmt.Sprintf("%q", s)
	}
	return strings.Join(q, sep)
}
