package cli

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/agent-sandbox/cli/internal/daemon"
	"github.com/agent-sandbox/cli/internal/manifest"
)

// TestManifestPayload_FieldsMatch asserts that manifest.Manifest and
// daemon.ManifestPayload expose the same JSON field set. manifestToPayload
// hand-copies fields between the two structs (run.go), so a mismatch here
// would silently drop data on the wire when either struct gains a field.
func TestManifestPayload_FieldsMatch(t *testing.T) {
	want := jsonFieldSet(reflect.TypeOf(manifest.Manifest{}))
	got := jsonFieldSet(reflect.TypeOf(daemon.ManifestPayload{}))

	if !reflect.DeepEqual(want, got) {
		t.Fatalf("JSON field set drift between manifest.Manifest and daemon.ManifestPayload\n  manifest.Manifest:        %v\n  daemon.ManifestPayload:   %v\n  hint: update manifestToPayload (internal/cli/run.go) to copy any new fields",
			want, got)
	}
}

// TestManifestToPayload_CopiesAllFields runs manifestToPayload against a
// fully-populated Manifest and checks every JSON field on the payload is
// non-zero. Catches a hand-copy regression where a new field is added to both
// structs but forgotten in the copy.
func TestManifestToPayload_CopiesAllFields(t *testing.T) {
	src := &manifest.Manifest{
		Name:         "agent-x",
		Command:      []string{"/bin/true"},
		AllowedHosts: []string{"api.example.com"},
		AllowedPaths: []string{"/etc/x"},
		WorkingDir:   "/tmp/x",
		Env:          map[string]string{"K": "V"},
		User:         "1000",
		Stdin:        "close",
		TimeoutNS:    30_000_000_000,
		Description:  "test",
	}
	got := manifestToPayload(src)

	v := reflect.ValueOf(got)
	tp := v.Type()
	for i := 0; i < tp.NumField(); i++ {
		f := tp.Field(i)
		tag, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if tag == "" || tag == "-" {
			continue
		}
		if v.Field(i).IsZero() {
			t.Errorf("manifestToPayload: field %q (json:%q) is zero — likely missing in the copy",
				f.Name, tag)
		}
	}
}

// jsonFieldSet returns the sorted set of JSON tag names defined on t.
func jsonFieldSet(t reflect.Type) []string {
	var out []string
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if tag == "" || tag == "-" {
			continue
		}
		out = append(out, tag)
	}
	sort.Strings(out)
	return out
}
