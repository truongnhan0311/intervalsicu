package intervalsicu_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/truongnhan0311/intervalsicu"
)

func TestWithHTTPClientIsUsed(t *testing.T) {
	// Callers need to supply their own transport for proxies, retries or
	// tracing. If the option is quietly ignored none of that takes effect and
	// nothing says so.
	used := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"i1"}`))
	}))
	t.Cleanup(server.Close)

	custom := &http.Client{
		Timeout: 5 * time.Second,
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			used = true

			return http.DefaultTransport.RoundTrip(r)
		}),
	}

	client := intervalsicu.New("k",
		intervalsicu.WithBaseURL(server.URL),
		intervalsicu.WithHTTPClient(custom))

	if _, err := client.Athlete(t.Context()); err != nil {
		t.Fatalf("Athlete: %v", err)
	}
	if !used {
		t.Error("the supplied http.Client was not used")
	}
}

func TestWithUserAgentIsSent(t *testing.T) {
	// Identifying your application is good manners against a small service and
	// makes your traffic recognisable if you ever need to ask about it.
	var got string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("User-Agent")
		_, _ = w.Write([]byte(`{"id":"i1"}`))
	}))
	t.Cleanup(server.Close)

	client := intervalsicu.New("k",
		intervalsicu.WithBaseURL(server.URL),
		intervalsicu.WithUserAgent("taper/1.0"))

	if _, err := client.Athlete(t.Context()); err != nil {
		t.Fatalf("Athlete: %v", err)
	}
	if got != "taper/1.0" {
		t.Errorf("User-Agent = %q, want taper/1.0", got)
	}
}

func TestDefaultUserAgentIdentifiesTheLibrary(t *testing.T) {
	var got string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("User-Agent")
		_, _ = w.Write([]byte(`{"id":"i1"}`))
	}))
	t.Cleanup(server.Close)

	client := intervalsicu.New("k", intervalsicu.WithBaseURL(server.URL))

	if _, err := client.Athlete(t.Context()); err != nil {
		t.Fatalf("Athlete: %v", err)
	}
	if got == "" {
		t.Error("a default User-Agent should be sent")
	}
}

func TestEmptyOptionValuesAreIgnored(t *testing.T) {
	// An empty string or nil usually means a config value was missing. Taking
	// it literally would point the client at "" and fail far from the cause.
	client := intervalsicu.New("k",
		intervalsicu.WithAthleteID(""),
		intervalsicu.WithBaseURL(""),
		intervalsicu.WithUserAgent(""),
		intervalsicu.WithHTTPClient(nil))

	if client.AthleteID() != intervalsicu.SelfAthlete {
		t.Errorf("AthleteID = %q, want the default %q", client.AthleteID(), intervalsicu.SelfAthlete)
	}
}

func TestBaseURLTrailingSlashIsTrimmed(t *testing.T) {
	// Otherwise every path becomes a double slash, which some servers answer
	// with a redirect and others with 404.
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = w.Write([]byte(`{"id":"i1"}`))
	}))
	t.Cleanup(server.Close)

	client := intervalsicu.New("k", intervalsicu.WithBaseURL(server.URL+"/"))

	if _, err := client.Athlete(t.Context()); err != nil {
		t.Fatalf("Athlete: %v", err)
	}
	if path != "/athlete/0" {
		t.Errorf("path = %q, want /athlete/0", path)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
