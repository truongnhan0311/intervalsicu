package intervalsicu_test

import (
	"encoding/json"
	"testing"

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
