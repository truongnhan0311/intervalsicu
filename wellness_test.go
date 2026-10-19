package intervalsicu_test

import (
	"testing"
)

func TestWellnessDecodesCamelCaseFields(t *testing.T) {
	// Wellness uses camelCase while activities use snake_case. That is the
	// API's inconsistency, and getting it wrong silently yields nil everywhere.
	client, _ := newFake(t, routes{
		"/athlete/0/wellness": `[
          {"id":"2026-10-14","restingHR":48,"hrv":62.5,"sleepSecs":27000,
           "sleepScore":81,"weight":70.2,"ctl":62.1}
        ]`,
	})

	records, err := client.Wellness(t.Context(), day(2026, 10, 1), day(2026, 10, 20))
	if err != nil {
		t.Fatalf("Wellness: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}

	record := records[0]
	if record.RestingHR == nil || *record.RestingHR != 48 {
		t.Error("restingHR not decoded")
	}
	if record.Hrv == nil || *record.Hrv != 62.5 {
		t.Error("hrv not decoded")
	}
	if record.SleepSecs == nil || *record.SleepSecs != 27000 {
		t.Error("sleepSecs not decoded")
	}

	date, err := record.Date()
	if err != nil {
		t.Fatalf("Date: %v", err)
	}
	if date.Format("2006-01-02") != "2026-10-14" {
		t.Errorf("Date() = %v", date)
	}
}

func TestWellnessDayFetchesASingleDate(t *testing.T) {
	client, seen := newFake(t, routes{
		"/athlete/0/wellness/2026-10-14": `{"id":"2026-10-14","restingHR":48}`,
	})

	record, err := client.WellnessDay(t.Context(), day(2026, 10, 14))
	if err != nil {
		t.Fatalf("WellnessDay: %v", err)
	}
	if record.ID != "2026-10-14" {
		t.Errorf("ID = %q", record.ID)
	}
	if _, ok := seen["/athlete/0/wellness/2026-10-14"]; !ok {
		t.Error("expected the date in the path")
	}
}
