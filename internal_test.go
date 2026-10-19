package intervalsicu

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

// jsonFieldNames feeds Extra: a name it misses would be decoded into its real
// field *and* copied into Extra, so the tag cases are worth pinning down.
func TestJsonFieldNamesHandlesEveryTagShape(t *testing.T) {
	type sample struct {
		Tagged   string `json:"tagged"`
		WithOpts string `json:"with_opts,omitempty"`
		Untagged string
		Skipped  string `json:"-"`
	}

	names := jsonFieldNames(reflect.TypeFor[sample]())

	for _, want := range []string{"tagged", "with_opts", "Untagged"} {
		if !names[want] {
			t.Errorf("want %q among the known names, got %v", want, names)
		}
	}
	if names["-"] || names["Skipped"] {
		t.Errorf("a json:\"-\" field must not be a known name, got %v", names)
	}
	if names["with_opts,omitempty"] {
		t.Errorf("tag options must be stripped from the name, got %v", names)
	}
}

// The Activity struct is the one this package actually decodes into; if Extra
// ever became a known name it would swallow a server field called "-".
func TestKnownActivityFieldsSkipsExtra(t *testing.T) {
	known := knownActivityFields()

	if !known["id"] || !known["icu_training_load"] {
		t.Errorf("real activity fields missing from the known set: %v", known)
	}
	if known["-"] {
		t.Error("Extra's json:\"-\" tag leaked into the known set")
	}
}

func TestGetAcceptsANilDestination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ignored":true}`))
	}))
	defer server.Close()

	client := New("secret-key", WithBaseURL(server.URL))

	if err := client.get(context.Background(), "/ping", nil, nil); err != nil {
		t.Fatalf("a nil destination should skip decoding, got %v", err)
	}
}

func TestGetReportsAnUnbuildableRequest(t *testing.T) {
	// A control character cannot appear in a request URL, so the failure
	// happens before any connection is attempted.
	client := New("secret-key", WithBaseURL("http://example.invalid"))

	err := client.get(context.Background(), "/activity/\x7f", nil, &struct{}{})
	if err == nil {
		t.Fatal("want an error for a path that cannot be put in a URL, got nil")
	}
	var apiErr *Error
	if asError(err, &apiErr) {
		t.Errorf("a malformed request is not an API error, got %v", err)
	}
}

// Extra holds raw JSON, so a caller can still reach a field this package has
// not mapped yet without the library having to guess its type.
func TestUnmarshalKeepsUnmappedFieldsRaw(t *testing.T) {
	var activity Activity

	if err := json.Unmarshal([]byte(`{"id":"i1","some_new_field":{"a":1}}`), &activity); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	raw, ok := activity.Extra["some_new_field"]
	if !ok {
		t.Fatalf("want the unmapped field kept, got %v", activity.Extra)
	}
	if string(raw) != `{"a":1}` {
		t.Errorf("want the raw JSON preserved, got %s", raw)
	}
	if _, ok := activity.Extra["id"]; ok {
		t.Error("a mapped field must not be duplicated into Extra")
	}
}
