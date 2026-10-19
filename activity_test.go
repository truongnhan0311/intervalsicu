package intervalsicu_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/truongnhan0311/intervalsicu"
)

func TestActivityStartPrefersUtcField(t *testing.T) {
	var activity intervalsicu.Activity
	mustUnmarshal(t, `{
      "id":"i1",
      "start_date":"2026-10-15T00:30:00Z",
      "start_date_local":"2026-10-15T07:30:00",
      "timezone":"Asia/Ho_Chi_Minh"
    }`, &activity)

	if got := activity.Start().Format("2006-01-02T15:04:05Z"); got != "2026-10-15T00:30:00Z" {
		t.Errorf("Start() = %s", got)
	}
}

func TestActivityStartFallsBackToLocalPlusTimezone(t *testing.T) {
	// Reading the local string as UTC shifts a ride by hours; east of
	// Greenwich it can move an evening session onto the previous day.
	var activity intervalsicu.Activity
	mustUnmarshal(t, `{
      "id":"i1",
      "start_date_local":"2026-10-15T07:30:00",
      "timezone":"Asia/Ho_Chi_Minh"
    }`, &activity)

	if got := activity.Start().Format("2006-01-02T15:04:05Z"); got != "2026-10-15T00:30:00Z" {
		t.Errorf("Start() = %s, want the local time converted to UTC", got)
	}
}

func TestHasRealPower(t *testing.T) {
	tests := []struct {
		name string
		json string
		want bool
	}{
		{"measured", `{"id":"i1","device_watts":true}`, true},
		{"estimated", `{"id":"i1","device_watts":false}`, false},
		// Absent means unknown, and unknown must read as estimated: an
		// estimate accepted as measurement poisons every load figure built on
		// it, and nothing downstream can tell afterwards.
		{"absent", `{"id":"i1"}`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var activity intervalsicu.Activity
			mustUnmarshal(t, tt.json, &activity)

			if got := activity.HasRealPower(); got != tt.want {
				t.Errorf("HasRealPower() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsIndoor(t *testing.T) {
	tests := []struct {
		name string
		json string
		want bool
	}{
		// The trainer flag is more reliable than the type string: a Zwift
		// session can arrive as a plain "Ride".
		{"trainer flag on a plain ride", `{"id":"i1","type":"Ride","trainer":true}`, true},
		{"virtual ride type", `{"id":"i1","type":"VirtualRide"}`, true},
		{"outdoor", `{"id":"i1","type":"Ride","trainer":false}`, false},
		{"outdoor without flag", `{"id":"i1","type":"GravelRide"}`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var activity intervalsicu.Activity
			mustUnmarshal(t, tt.json, &activity)

			if got := activity.IsIndoor(); got != tt.want {
				t.Errorf("IsIndoor() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUnmappedFieldsLandInExtra(t *testing.T) {
	// The API returns well over a hundred fields and keeps growing. Anything
	// not modelled stays reachable rather than disappearing.
	var activity intervalsicu.Activity
	mustUnmarshal(t, `{
      "id":"i1","type":"Ride",
      "icu_rpe": 7,
      "some_future_field": {"nested": true}
    }`, &activity)

	if activity.ID != "i1" {
		t.Errorf("ID = %q", activity.ID)
	}
	if _, ok := activity.Extra["icu_rpe"]; !ok {
		t.Error("icu_rpe should be preserved in Extra")
	}
	if _, ok := activity.Extra["some_future_field"]; !ok {
		t.Error("unknown fields should be preserved in Extra")
	}
	// Mapped fields must not be duplicated into Extra.
	if _, ok := activity.Extra["id"]; ok {
		t.Error("id is a modelled field and should not appear in Extra")
	}
	if _, ok := activity.Extra["type"]; ok {
		t.Error("type is a modelled field and should not appear in Extra")
	}
}

func TestAbsentNumbersStayNil(t *testing.T) {
	// nil and zero are different answers: a ride with no power meter has no
	// average watts, while a ride spent coasting really does average zero.
	var activity intervalsicu.Activity
	mustUnmarshal(t, `{"id":"i1","type":"Ride"}`, &activity)

	if activity.IcuAverageWatts != nil {
		t.Error("IcuAverageWatts should be nil when absent")
	}
	if activity.AverageHeartrate != nil {
		t.Error("AverageHeartrate should be nil when absent")
	}

	mustUnmarshal(t, `{"id":"i1","icu_average_watts":0}`, &activity)
	if activity.IcuAverageWatts == nil || *activity.IcuAverageWatts != 0 {
		t.Error("an explicit zero must survive as zero, not nil")
	}
}

func TestActivitiesDecodesAList(t *testing.T) {
	client, _ := newFake(t, routes{
		"/athlete/0/activities": `[
          {"id":"i1","type":"Ride","elapsed_time":3600,"device_watts":true,
           "icu_weighted_avg_watts":250},
          {"id":"i2","type":"Run","elapsed_time":1800}
        ]`,
	})

	list, err := client.Activities(t.Context(), day(2026, 9, 1), day(2026, 10, 1))
	if err != nil {
		t.Fatalf("Activities: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("got %d activities, want 2", len(list))
	}
	if !list[0].HasRealPower() {
		t.Error("first activity should report real power")
	}
	if list[0].IcuWeightedAvgWatts == nil || *list[0].IcuWeightedAvgWatts != 250 {
		t.Error("normalised power not decoded")
	}
}

func mustUnmarshal(t *testing.T, data string, dest any) {
	t.Helper()

	if err := json.Unmarshal([]byte(data), dest); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
}

// An activity with no usable start is better as a zero time than as a
// confidently wrong one: a caller can test for it, a silently shifted date
// just lands the ride on the wrong day of the training log.
func TestStartIsZeroWhenThereIsNothingToParse(t *testing.T) {
	cases := map[string]intervalsicu.Activity{
		"no dates at all":        {ID: "i1"},
		"only an unusable UTC":   {ID: "i2", StartDate: "last tuesday"},
		"unparseable local time": {ID: "i3", StartDateLocal: "yesterday evening", Timezone: "Europe/Paris"},
	}

	for name, activity := range cases {
		t.Run(name, func(t *testing.T) {
			if got := activity.Start(); !got.IsZero() {
				t.Errorf("want the zero time, got %s", got)
			}
		})
	}
}

// A bad UTC stamp must not stop the local time from being used.
func TestStartFallsBackToLocalWhenUtcIsUnusable(t *testing.T) {
	activity := intervalsicu.Activity{
		StartDate:      "not a timestamp",
		StartDateLocal: "2026-03-14T18:30:00",
		Timezone:       "Asia/Ho_Chi_Minh",
	}

	want := time.Date(2026, 3, 14, 11, 30, 0, 0, time.UTC)
	if got := activity.Start(); !got.Equal(want) {
		t.Errorf("want %s, got %s", want, got)
	}
}

// An unknown timezone name falls back to UTC rather than failing outright.
func TestStartTreatsAnUnknownTimezoneAsUtc(t *testing.T) {
	activity := intervalsicu.Activity{
		StartDateLocal: "2026-03-14T18:30:00",
		Timezone:       "Mars/Olympus_Mons",
	}

	want := time.Date(2026, 3, 14, 18, 30, 0, 0, time.UTC)
	if got := activity.Start(); !got.Equal(want) {
		t.Errorf("want %s, got %s", want, got)
	}
}

func TestActivityPropagatesAFetchError(t *testing.T) {
	client, _ := newFake(t, routes{})

	_, err := client.Activity(context.Background(), "i-missing")
	if err == nil {
		t.Fatal("want an error for an activity the server does not have, got nil")
	}
	if !intervalsicu.IsNotFound(err) {
		t.Errorf("want a not-found error, got %v", err)
	}
}

func TestActivityRejectsAnEmptyID(t *testing.T) {
	client, seen := newFake(t, routes{})

	if _, err := client.Activity(context.Background(), ""); err == nil {
		t.Fatal("want an error for an empty id, got nil")
	}
	if len(seen) != 0 {
		t.Errorf("an empty id must not reach the server, got %v", seen)
	}
}

// A field of the wrong type must come back as an error, not a panic or a
// half-filled activity.
func TestActivityUnmarshalRejectsAMistypedField(t *testing.T) {
	var activity intervalsicu.Activity

	if err := json.Unmarshal([]byte(`{"id":123}`), &activity); err == nil {
		t.Fatal("want a decode error for a numeric id, got nil")
	}
}

func TestActivityFetchesOneRide(t *testing.T) {
	client, seen := newFake(t, routes{
		"/activity/i123": `{
          "id":"i123",
          "type":"Ride",
          "icu_training_load":82,
          "device_watts":true
        }`,
	})

	activity, err := client.Activity(context.Background(), "i123")
	if err != nil {
		t.Fatalf("Activity: %v", err)
	}
	if activity.ID != "i123" {
		t.Errorf("want id i123, got %q", activity.ID)
	}
	if !activity.HasRealPower() {
		t.Error("device_watts true must read as real power")
	}
	if _, ok := seen["/activity/i123"]; !ok {
		t.Errorf("want the activity endpoint called, got %v", seen)
	}
}

// An id with a slash in it must stay inside the activity path instead of
// walking up to another endpoint.
func TestActivityEscapesTheIDInThePath(t *testing.T) {
	client, seen := newFake(t, routes{})

	_, _ = client.Activity(context.Background(), "i1/../athlete")

	if _, ok := seen["/athlete"]; ok {
		t.Fatalf("the id walked out of the activity path, got %v", seen)
	}
	request, ok := seen["/activity/i1/../athlete"]
	if !ok {
		t.Fatalf("want the activity endpoint called, got %v", seen)
	}
	// Go only leaves the dot segments in place because they arrived escaped;
	// an unescaped id would have been collapsed before the request was sent.
	if escaped := request.URL.EscapedPath(); escaped != "/activity/i1%2F..%2Fathlete" {
		t.Errorf("want the id escaped into one path segment, got %q", escaped)
	}
}
