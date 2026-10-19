package intervalsicu_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/truongnhan0311/intervalsicu"
)

func TestErrorHelpersIgnoreUnrelatedErrors(t *testing.T) {
	// A network failure is not a 401. Reporting it as one sends callers off to
	// re-check a credential that was never the problem.
	unrelated := errors.New("dial tcp: connection refused")

	if intervalsicu.IsUnauthorized(unrelated) {
		t.Error("IsUnauthorized should be false for a non-API error")
	}
	if intervalsicu.IsNotFound(unrelated) {
		t.Error("IsNotFound should be false for a non-API error")
	}
	if intervalsicu.IsUnauthorized(nil) || intervalsicu.IsNotFound(nil) {
		t.Error("nil is not an error")
	}
}

func TestErrorIsUnwrappable(t *testing.T) {
	// Callers should be able to reach the status code for cases the two
	// helpers do not cover, such as 429.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(server.Close)

	client := intervalsicu.New("k", intervalsicu.WithBaseURL(server.URL))

	_, err := client.Athlete(t.Context())

	var apiErr *intervalsicu.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("errors.As failed for %v", err)
	}
	if apiErr.StatusCode != http.StatusTooManyRequests {
		t.Errorf("StatusCode = %d, want 429", apiErr.StatusCode)
	}
	if apiErr.Method != http.MethodGet {
		t.Errorf("Method = %q, want GET", apiErr.Method)
	}
	if apiErr.Path == "" {
		t.Error("Path should name the endpoint that failed")
	}
}

func TestUnauthorizedCoversBoth401And403(t *testing.T) {
	// intervals.icu answers a bad key with 403, not 401, so branching on 401
	// alone misses the common case.
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
			}))
			t.Cleanup(server.Close)

			client := intervalsicu.New("k", intervalsicu.WithBaseURL(server.URL))

			_, err := client.Athlete(t.Context())
			if !intervalsicu.IsUnauthorized(err) {
				t.Errorf("IsUnauthorized for %d = false, want true", status)
			}
		})
	}
}

func TestMalformedJsonIsReportedNotPanicked(t *testing.T) {
	client, _ := newFake(t, routes{"/athlete/0": `{"id": `})

	if _, err := client.Athlete(t.Context()); err == nil {
		t.Error("expected a decode error")
	}
}

func TestContextCancellationIsHonoured(t *testing.T) {
	// A sync job that cannot be cancelled holds whatever it is holding until
	// it decides to finish on its own.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)

	client := intervalsicu.New("k", intervalsicu.WithBaseURL(server.URL))

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	if _, err := client.Athlete(ctx); err == nil {
		t.Error("expected the request to fail once the context expired")
	}
}

func TestTransportFailureIsWrapped(t *testing.T) {
	// Point at a server that is already closed.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := server.URL
	server.Close()

	client := intervalsicu.New("k", intervalsicu.WithBaseURL(url))

	_, err := client.Athlete(t.Context())
	if err == nil {
		t.Fatal("expected a transport error")
	}
	// Not an API error — there was no response to carry a status.
	var apiErr *intervalsicu.Error
	if errors.As(err, &apiErr) {
		t.Error("a transport failure should not masquerade as an API status error")
	}
}

func TestListEndpointsPropagateErrors(t *testing.T) {
	// Every list call must surface a failure rather than returning an empty
	// slice, which reads as "this athlete has no data".
	client, _ := newFake(t, routes{})
	oldest, newest := day(2026, 9, 1), day(2026, 10, 1)

	if _, err := client.Activities(t.Context(), oldest, newest); err == nil {
		t.Error("Activities should surface a 404")
	}
	if _, err := client.Wellness(t.Context(), oldest, newest); err == nil {
		t.Error("Wellness should surface a 404")
	}
	if _, err := client.ActivityStreams(t.Context(), "i1"); err == nil {
		t.Error("ActivityStreams should surface a 404")
	}
	if _, err := client.WellnessDay(t.Context(), oldest); err == nil {
		t.Error("WellnessDay should surface a 404")
	}
}

func TestWellnessDayRejectsAnEmptyRecord(t *testing.T) {
	// A 200 carrying {} means the day has no record. Returning a zero-valued
	// Wellness would read as a real day with every field unset.
	client, _ := newFake(t, routes{"/athlete/0/wellness/2026-10-14": `{}`})

	if _, err := client.WellnessDay(t.Context(), day(2026, 10, 14)); err == nil {
		t.Error("expected an error for an empty wellness record")
	}
}
