package intervalsicu

import (
	"context"
	"fmt"
	"net/url"
	"time"
)

// Wellness is one day of health and readiness data.
//
// Fields use camelCase because that is what the API returns here; activity
// fields use snake_case. The inconsistency is upstream, not a mistake below.
//
// Most fields are optional and stay nil unless a device supplied them or the
// athlete entered them by hand.
type Wellness struct {
	// ID is the calendar date as "2006-01-02", not a number.
	ID      string `json:"id"`
	Updated string `json:"updated,omitempty"`

	// Ctl, Atl and RampRate are intervals.icu's own fitness model. Useful on
	// their own; best not mixed into figures you compute yourself, since they
	// carry its thresholds and its decay constants.
	Ctl      *float64 `json:"ctl,omitempty"`
	Atl      *float64 `json:"atl,omitempty"`
	RampRate *float64 `json:"rampRate,omitempty"`
	CtlLoad  *float64 `json:"ctlLoad,omitempty"`
	AtlLoad  *float64 `json:"atlLoad,omitempty"`

	Weight    *float64 `json:"weight,omitempty"`
	RestingHR *int     `json:"restingHR,omitempty"`
	// Hrv is the nightly average, usually rMSSD.
	Hrv     *float64 `json:"hrv,omitempty"`
	HrvSDNN *float64 `json:"hrvSDNN,omitempty"`

	SleepSecs     *int     `json:"sleepSecs,omitempty"`
	SleepScore    *float64 `json:"sleepScore,omitempty"`
	SleepQuality  *int     `json:"sleepQuality,omitempty"`
	AvgSleepingHR *float64 `json:"avgSleepingHR,omitempty"`

	// Subjective entries, scored by the athlete.
	Soreness   *int `json:"soreness,omitempty"`
	Fatigue    *int `json:"fatigue,omitempty"`
	Stress     *int `json:"stress,omitempty"`
	Mood       *int `json:"mood,omitempty"`
	Motivation *int `json:"motivation,omitempty"`
	Injury     *int `json:"injury,omitempty"`

	SpO2         *float64 `json:"spO2,omitempty"`
	Systolic     *int     `json:"systolic,omitempty"`
	Diastolic    *int     `json:"diastolic,omitempty"`
	Hydration    *int     `json:"hydration,omitempty"`
	Readiness    *float64 `json:"readiness,omitempty"`
	BloodGlucose *float64 `json:"bloodGlucose,omitempty"`
	Lactate      *float64 `json:"lactate,omitempty"`
	BodyFat      *float64 `json:"bodyFat,omitempty"`
	VO2Max       *float64 `json:"vo2max,omitempty"`
	KcalConsumed *int     `json:"kcalConsumed,omitempty"`
	Comments     string   `json:"comments,omitempty"`
}

// Date parses [Wellness.ID] into a date.
func (w Wellness) Date() (time.Time, error) {
	return time.Parse(time.DateOnly, w.ID)
}

// Wellness lists daily wellness records between two dates, inclusive.
func (c *Client) Wellness(ctx context.Context, oldest, newest time.Time) ([]Wellness, error) {
	var records []Wellness
	if err := c.get(ctx, c.athletePath("/wellness"), dateRange(oldest, newest), &records); err != nil {
		return nil, err
	}

	return records, nil
}

// WellnessDay fetches one day's wellness record.
func (c *Client) WellnessDay(ctx context.Context, day time.Time) (*Wellness, error) {
	date := day.Format(time.DateOnly)

	var record Wellness
	if err := c.get(ctx, c.athletePath("/wellness/"+url.PathEscape(date)), nil, &record); err != nil {
		return nil, err
	}
	if record.ID == "" {
		return nil, fmt.Errorf("intervals.icu: no wellness record for %s", date)
	}

	return &record, nil
}
