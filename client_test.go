package intervalsicu_test

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/truongnhan0311/intervalsicu"
)

// routes maps a request path to the JSON body the fake server returns.
type routes map[string]string

// newFake starts a stub intervals.icu and returns a client pointed at it,
// together with the requests it received.
func newFake(t *testing.T, handlers routes) (*intervalsicu.Client, map[string]*http.Request) {
	t.Helper()

	seen := map[string]*http.Request{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen[r.URL.Path] = r.Clone(r.Context())

		body, ok := handlers[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)

			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	return intervalsicu.New("secret-key", intervalsicu.WithBaseURL(server.URL)), seen
}

func TestBasicAuthUsesLiteralApiKeyUsername(t *testing.T) {
	// The single most common mistake against this API: the username is the
	// fixed string "API_KEY" and the key is the password. Swapped, the server
	// answers 403 and says nothing about why.
	client, seen := newFake(t, routes{"/athlete/0": `{"id":"i123456","name":"Test"}`})

	if _, err := client.Athlete(t.Context()); err != nil {
		t.Fatalf("Athlete: %v", err)
	}

	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("API_KEY:secret-key"))
	if got := seen["/athlete/0"].Header.Get("Authorization"); got != want {
		t.Errorf("Authorization = %q, want %q", got, want)
	}
}

func TestBearerTokenTakesPrecedence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer oauth-token" {
			t.Errorf("Authorization = %q, want bearer token", got)
		}
		_, _ = w.Write([]byte(`{"id":"i1"}`))
	}))
	t.Cleanup(server.Close)

	client := intervalsicu.New("", intervalsicu.WithBaseURL(server.URL),
		intervalsicu.WithBearerToken("oauth-token"))

	if _, err := client.Athlete(t.Context()); err != nil {
		t.Fatalf("Athlete: %v", err)
	}
}

func TestDefaultAthleteIsSelf(t *testing.T) {
	// "0" means whoever owns the credential, so the common case needs no id.
	client, seen := newFake(t, routes{"/athlete/0": `{"id":"i1"}`})

	if client.AthleteID() != intervalsicu.SelfAthlete {
		t.Errorf("AthleteID = %q, want %q", client.AthleteID(), intervalsicu.SelfAthlete)
	}
	if _, err := client.Athlete(t.Context()); err != nil {
		t.Fatalf("Athlete: %v", err)
	}
	if _, ok := seen["/athlete/0"]; !ok {
		t.Error("expected request to /athlete/0")
	}
}

func TestWithAthleteIDOverridesPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/athlete/i999" {
			t.Errorf("path = %q, want /athlete/i999", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"id":"i999"}`))
	}))
	t.Cleanup(server.Close)

	client := intervalsicu.New("k", intervalsicu.WithBaseURL(server.URL),
		intervalsicu.WithAthleteID("i999"))

	if _, err := client.Athlete(t.Context()); err != nil {
		t.Fatalf("Athlete: %v", err)
	}
}

func TestErrorsCarryStatusWithoutLeakingTheBody(t *testing.T) {
	// An error from an auth endpoint can echo the credential just sent, and
	// errors end up in logs.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"bad key","key":"secret-key"}`))
	}))
	t.Cleanup(server.Close)

	client := intervalsicu.New("secret-key", intervalsicu.WithBaseURL(server.URL))

	_, err := client.Athlete(t.Context())
	if err == nil {
		t.Fatal("expected an error")
	}
	if !intervalsicu.IsUnauthorized(err) {
		t.Errorf("IsUnauthorized(%v) = false, want true", err)
	}
	if got := err.Error(); contains(got, "secret-key") {
		t.Errorf("error message leaks the credential: %q", got)
	}
}

func TestIsNotFound(t *testing.T) {
	client, _ := newFake(t, routes{})

	_, err := client.Activity(t.Context(), "missing")
	if !intervalsicu.IsNotFound(err) {
		t.Errorf("IsNotFound(%v) = false, want true", err)
	}
	if intervalsicu.IsUnauthorized(err) {
		t.Error("a 404 must not read as unauthorized")
	}
}

func TestListEndpointsSendDateRange(t *testing.T) {
	client, seen := newFake(t, routes{
		"/athlete/0/activities": `[]`,
		"/athlete/0/wellness":   `[]`,
	})
	oldest := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	newest := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

	if _, err := client.Activities(t.Context(), oldest, newest); err != nil {
		t.Fatalf("Activities: %v", err)
	}
	if _, err := client.Wellness(t.Context(), oldest, newest); err != nil {
		t.Fatalf("Wellness: %v", err)
	}

	for _, path := range []string{"/athlete/0/activities", "/athlete/0/wellness"} {
		query := seen[path].URL.Query()
		if query.Get("oldest") != "2026-09-01" || query.Get("newest") != "2026-10-01" {
			t.Errorf("%s query = %v", path, query)
		}
	}
}

func TestEmptyIdsAreRejectedBeforeAnyRequest(t *testing.T) {
	client, seen := newFake(t, routes{})

	if _, err := client.Activity(t.Context(), ""); err == nil {
		t.Error("Activity(\"\") should fail")
	}
	if _, err := client.ActivityStreams(t.Context(), ""); err == nil {
		t.Error("ActivityStreams(\"\") should fail")
	}
	if len(seen) != 0 {
		t.Errorf("made %d requests for empty ids, want 0", len(seen))
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}

	return -1
}
