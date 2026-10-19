package intervalsicu_test

import (
	"math"
	"sort"
	"testing"
	"time"

	"github.com/truongnhan0311/intervalsicu"
)

func day(year int, month time.Month, d int) time.Time {
	return time.Date(year, month, d, 0, 0, 0, 0, time.UTC)
}

func TestStreamsArrayShape(t *testing.T) {
	client, _ := newFake(t, routes{
		"/activity/i1/streams": `[
          {"type":"watts","data":[200,210,null,195]},
          {"type":"heartrate","data":[140,141,142,143]}
        ]`,
	})

	streams, err := client.ActivityStreams(t.Context(), "i1")
	if err != nil {
		t.Fatalf("ActivityStreams: %v", err)
	}

	watts, ok := streams.Get(intervalsicu.StreamWatts)
	if !ok {
		t.Fatal("watts stream missing")
	}
	if watts.Len() != 4 {
		t.Fatalf("watts length = %d, want 4", watts.Len())
	}
	if watts.Data[2] != nil {
		t.Error("a null sample must stay nil, not become zero")
	}
}

func TestStreamsObjectShape(t *testing.T) {
	// The API has been observed returning both shapes. Guessing one and
	// returning nothing for the other looks exactly like an activity with no
	// streams at all — a failure nobody notices until the numbers are wrong.
	client, _ := newFake(t, routes{
		"/activity/i1/streams": `{"watts":[200,210],"heartrate":[140,141]}`,
	})

	streams, err := client.ActivityStreams(t.Context(), "i1")
	if err != nil {
		t.Fatalf("ActivityStreams: %v", err)
	}
	if len(streams) != 2 {
		t.Fatalf("got %d streams, want 2", len(streams))
	}
	if watts, _ := streams.Get("watts"); watts.Len() != 2 {
		t.Errorf("watts length = %d, want 2", watts.Len())
	}
}

func TestStreamsRequestsOnlyWhatWasAsked(t *testing.T) {
	// A long ride carries tens of thousands of points per channel, so asking
	// for everything when two channels are needed is not free.
	client, seen := newFake(t, routes{"/activity/i1/streams": `[]`})

	if _, err := client.ActivityStreams(t.Context(), "i1",
		intervalsicu.StreamWatts, intervalsicu.StreamHeartrate); err != nil {
		t.Fatalf("ActivityStreams: %v", err)
	}

	if got := seen["/activity/i1/streams"].URL.Query().Get("types"); got != "watts,heartrate" {
		t.Errorf("types = %q, want watts,heartrate", got)
	}
}

func TestStreamIntsAndFloatsSubstituteMissing(t *testing.T) {
	client, _ := newFake(t, routes{
		"/activity/i1/streams": `[{"type":"watts","data":[200,null,195.6]}]`,
	})

	streams, err := client.ActivityStreams(t.Context(), "i1")
	if err != nil {
		t.Fatalf("ActivityStreams: %v", err)
	}
	watts, _ := streams.Get("watts")

	ints := watts.Ints(-1)
	if ints[0] != 200 || ints[1] != -1 || ints[2] != 196 {
		t.Errorf("Ints(-1) = %v, want [200 -1 196]", ints)
	}

	floats := watts.Floats(math.NaN())
	if !math.IsNaN(floats[1]) {
		t.Error("Floats(NaN) should leave gaps detectable")
	}
}

func TestStreamCoverage(t *testing.T) {
	// A ride whose sensor dropped out for half its duration still produces a
	// perfectly ordinary looking average.
	client, _ := newFake(t, routes{
		"/activity/i1/streams": `[{"type":"watts","data":[200,null,195,null]}]`,
	})

	streams, err := client.ActivityStreams(t.Context(), "i1")
	if err != nil {
		t.Fatalf("ActivityStreams: %v", err)
	}
	watts, _ := streams.Get("watts")

	if got := watts.Coverage(); got != 0.5 {
		t.Errorf("Coverage() = %v, want 0.5", got)
	}
}

func TestStreamsRejectsUnknownShape(t *testing.T) {
	client, _ := newFake(t, routes{"/activity/i1/streams": `"not a stream"`})

	if _, err := client.ActivityStreams(t.Context(), "i1"); err == nil {
		t.Error("expected an error for an unrecognised response shape")
	}
}

func TestStreamsNamesListsChannels(t *testing.T) {
	client, _ := newFake(t, routes{
		"/activity/i1/streams": `[{"type":"watts","data":[1]},{"type":"heartrate","data":[2]}]`,
	})

	streams, err := client.ActivityStreams(t.Context(), "i1")
	if err != nil {
		t.Fatalf("ActivityStreams: %v", err)
	}

	names := streams.Names()
	if len(names) != 2 {
		t.Fatalf("Names() = %v, want two entries", names)
	}
	sort.Strings(names)
	if names[0] != "heartrate" || names[1] != "watts" {
		t.Errorf("Names() = %v", names)
	}
}

func TestStreamsGetReportsAbsence(t *testing.T) {
	// Distinguishing "no such channel" from "a channel of zero samples"
	// matters: the first means the device never recorded it.
	client, _ := newFake(t, routes{
		"/activity/i1/streams": `[{"type":"watts","data":[1]}]`,
	})

	streams, err := client.ActivityStreams(t.Context(), "i1")
	if err != nil {
		t.Fatalf("ActivityStreams: %v", err)
	}

	if _, ok := streams.Get("cadence"); ok {
		t.Error("Get should report a missing channel as absent")
	}
	if _, ok := streams.Get(intervalsicu.StreamWatts); !ok {
		t.Error("Get should find a present channel")
	}
}

func TestEmptyStreamHelpersDoNotPanic(t *testing.T) {
	var empty intervalsicu.Stream

	if empty.Len() != 0 {
		t.Error("Len on a zero Stream should be 0")
	}
	if got := empty.Coverage(); got != 0 {
		t.Errorf("Coverage on a zero Stream = %v, want 0", got)
	}
	if got := empty.Ints(-1); len(got) != 0 {
		t.Errorf("Ints on a zero Stream = %v, want empty", got)
	}
	if got := empty.Floats(0); len(got) != 0 {
		t.Errorf("Floats on a zero Stream = %v, want empty", got)
	}
}

func TestStreamIntsRoundsNegativesAwayFromZero(t *testing.T) {
	// Altitude below sea level and temperature both go negative; truncating
	// toward zero would bias every such sample upward.
	client, _ := newFake(t, routes{
		"/activity/i1/streams": `[{"type":"altitude","data":[-3.6,-3.4,3.6]}]`,
	})

	streams, err := client.ActivityStreams(t.Context(), "i1")
	if err != nil {
		t.Fatalf("ActivityStreams: %v", err)
	}
	altitude, _ := streams.Get("altitude")

	got := altitude.Ints(0)
	if got[0] != -4 || got[1] != -3 || got[2] != 4 {
		t.Errorf("Ints = %v, want [-4 -3 4]", got)
	}
}

func TestStreamsSkipEntriesWithoutAType(t *testing.T) {
	client, _ := newFake(t, routes{
		"/activity/i1/streams": `[{"data":[1,2]},{"type":"watts","data":[3]}]`,
	})

	streams, err := client.ActivityStreams(t.Context(), "i1")
	if err != nil {
		t.Fatalf("ActivityStreams: %v", err)
	}
	if len(streams) != 1 {
		t.Errorf("got %d streams, want 1 — an entry with no type names nothing", len(streams))
	}
}

func TestStreamsWithoutTypesFilterSendsNoQuery(t *testing.T) {
	client, seen := newFake(t, routes{"/activity/i1/streams": `[]`})

	if _, err := client.ActivityStreams(t.Context(), "i1"); err != nil {
		t.Fatalf("ActivityStreams: %v", err)
	}
	if got := seen["/activity/i1/streams"].URL.RawQuery; got != "" {
		t.Errorf("query = %q, want empty so the server chooses", got)
	}
}
