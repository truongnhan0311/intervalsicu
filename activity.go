package intervalsicu

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Activity is one workout.
//
// intervals.icu returns well over a hundred fields per activity. The struct
// below covers the ones most callers need; anything else is preserved in
// [Activity.Extra] so no data is silently lost when the API grows.
//
// Pointer fields are nil when the API omitted them, which is different from a
// zero value: a ride with no power meter has no average watts at all, while a
// ride spent coasting really does average zero.
type Activity struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
	// Type follows Strava's naming: Ride, VirtualRide, GravelRide, Run, ...
	Type string `json:"type,omitempty"`
	// Source names the upstream the activity arrived from: GARMIN, STRAVA, ...
	Source     string `json:"source,omitempty"`
	ExternalID string `json:"external_id,omitempty"`
	StravaID   string `json:"strava_id,omitempty"`

	// StartDate is the UTC instant; StartDateLocal is wall-clock time where
	// the ride happened, with no offset attached. Prefer StartDate — see
	// [Activity.Start].
	StartDate      string `json:"start_date,omitempty"`
	StartDateLocal string `json:"start_date_local,omitempty"`
	Timezone       string `json:"timezone,omitempty"`

	ElapsedTime        int      `json:"elapsed_time,omitempty"`
	MovingTime         *int     `json:"moving_time,omitempty"`
	CoastingTime       *int     `json:"coasting_time,omitempty"`
	Distance           *float64 `json:"distance,omitempty"`
	TotalElevationGain *float64 `json:"total_elevation_gain,omitempty"`
	Calories           *int     `json:"calories,omitempty"`

	// DeviceWatts reports whether power was measured by a real meter.
	//
	// When a ride has no power meter, Strava fills in an estimate derived from
	// speed, gradient and weight, and intervals.icu passes that number through
	// with DeviceWatts false. Treat estimated power as absent for anything
	// that feeds training load: it is a model's guess, not a measurement.
	DeviceWatts *bool `json:"device_watts,omitempty"`
	// Trainer marks an indoor ride. More reliable than Type, since a Zwift
	// session can arrive as a plain "Ride".
	Trainer *bool `json:"trainer,omitempty"`
	Commute *bool `json:"commute,omitempty"`
	Race    *bool `json:"race,omitempty"`

	IcuAverageWatts *float64 `json:"icu_average_watts,omitempty"`
	// IcuWeightedAvgWatts is normalised power. Unlike average watts it
	// reflects the cost of surging, which is why training load is built on it.
	IcuWeightedAvgWatts *float64 `json:"icu_weighted_avg_watts,omitempty"`
	IcuVariabilityIndex *float64 `json:"icu_variability_index,omitempty"`
	IcuEfficiencyFactor *float64 `json:"icu_efficiency_factor,omitempty"`
	// IcuPowerHr is Pw:HR decoupling as a percentage.
	IcuPowerHr *float64 `json:"icu_power_hr,omitempty"`
	IcuJoules  *float64 `json:"icu_joules,omitempty"`

	// IcuTrainingLoad, IcuIntensity, IcuCtl and IcuAtl are intervals.icu's own
	// computed figures. They are convenient, but they come from its model and
	// its thresholds — mixing them with figures you compute yourself produces
	// a chart nobody can reconcile later.
	IcuTrainingLoad *float64 `json:"icu_training_load,omitempty"`
	IcuIntensity    *float64 `json:"icu_intensity,omitempty"`
	IcuCtl          *float64 `json:"icu_ctl,omitempty"`
	IcuAtl          *float64 `json:"icu_atl,omitempty"`

	IcuFtp         *float64   `json:"icu_ftp,omitempty"`
	IcuWeight      *float64   `json:"icu_weight,omitempty"`
	IcuRestingHr   *float64   `json:"icu_resting_hr,omitempty"`
	Lthr           *float64   `json:"lthr,omitempty"`
	AthleteMaxHr   *float64   `json:"athlete_max_hr,omitempty"`
	IcuZoneTimes   []ZoneTime `json:"icu_zone_times,omitempty"`
	IcuHrZoneTimes []int      `json:"icu_hr_zone_times,omitempty"`
	IcuPowerZones  []float64  `json:"icu_power_zones,omitempty"`
	IcuHrZones     []float64  `json:"icu_hr_zones,omitempty"`

	AverageHeartrate *float64 `json:"average_heartrate,omitempty"`
	MaxHeartrate     *float64 `json:"max_heartrate,omitempty"`
	HasHeartrate     *bool    `json:"has_heartrate,omitempty"`
	AverageCadence   *float64 `json:"average_cadence,omitempty"`
	AverageSpeed     *float64 `json:"average_speed,omitempty"`
	MaxSpeed         *float64 `json:"max_speed,omitempty"`
	AverageTemp      *float64 `json:"average_temp,omitempty"`
	Pace             *float64 `json:"pace,omitempty"`

	DeviceName  string   `json:"device_name,omitempty"`
	PowerMeter  string   `json:"power_meter,omitempty"`
	Gear        string   `json:"gear,omitempty"`
	Description string   `json:"description,omitempty"`
	StreamTypes []string `json:"stream_types,omitempty"`

	// Extra holds every field not named above, so a schema change on the
	// server side never costs you data you were not expecting to need.
	Extra map[string]json.RawMessage `json:"-"`
}

// ZoneTime is seconds spent in one power or heart-rate zone.
type ZoneTime struct {
	ID   string `json:"id,omitempty"`
	Secs int    `json:"secs,omitempty"`
}

// activityAlias avoids infinite recursion in UnmarshalJSON.
type activityAlias Activity

// UnmarshalJSON decodes an activity and keeps unmapped fields in Extra.
func (a *Activity) UnmarshalJSON(data []byte) error {
	var alias activityAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	*a = Activity(alias)

	var all map[string]json.RawMessage
	if err := json.Unmarshal(data, &all); err != nil {
		return err
	}

	known := knownActivityFields()
	for name, value := range all {
		if known[name] {
			continue
		}
		if a.Extra == nil {
			a.Extra = map[string]json.RawMessage{}
		}
		a.Extra[name] = value
	}

	return nil
}

// Start returns the activity's start as an instant.
//
// StartDate is already UTC and is used when present. Otherwise the local
// wall-clock time is combined with the named timezone; reading that local
// string as UTC shifts a ride by hours, and for anyone east of Greenwich it
// can move an evening session onto the previous day.
func (a Activity) Start() time.Time {
	if a.StartDate != "" {
		if parsed, err := time.Parse(time.RFC3339, a.StartDate); err == nil {
			return parsed.UTC()
		}
	}
	if a.StartDateLocal == "" {
		return time.Time{}
	}

	location := time.UTC
	if a.Timezone != "" {
		if loaded, err := time.LoadLocation(a.Timezone); err == nil {
			location = loaded
		}
	}

	for _, layout := range []string{"2006-01-02T15:04:05", time.RFC3339} {
		if parsed, err := time.ParseInLocation(layout, a.StartDateLocal, location); err == nil {
			return parsed.UTC()
		}
	}

	return time.Time{}
}

// HasRealPower reports whether the activity's watts came from a power meter
// rather than Strava's estimate.
//
// A missing DeviceWatts counts as estimated. That is the cautious reading: an
// estimate accepted as measurement quietly poisons every training-load figure
// derived from it, and nothing downstream can tell the difference.
func (a Activity) HasRealPower() bool {
	return a.DeviceWatts != nil && *a.DeviceWatts
}

// IsIndoor reports whether the ride was on a trainer.
func (a Activity) IsIndoor() bool {
	if a.Trainer != nil && *a.Trainer {
		return true
	}

	return strings.EqualFold(a.Type, "VirtualRide") || strings.EqualFold(a.Type, "VirtualRun")
}

// Activities lists activities between two dates, inclusive.
func (c *Client) Activities(ctx context.Context, oldest, newest time.Time) ([]Activity, error) {
	var activities []Activity
	if err := c.get(ctx, c.athletePath("/activities"), dateRange(oldest, newest), &activities); err != nil {
		return nil, err
	}

	return activities, nil
}

// Activity fetches one activity by id.
func (c *Client) Activity(ctx context.Context, id string) (*Activity, error) {
	if id == "" {
		return nil, fmt.Errorf("intervals.icu: empty activity id")
	}

	var activity Activity
	if err := c.get(ctx, "/activity/"+url.PathEscape(id), nil, &activity); err != nil {
		return nil, err
	}

	return &activity, nil
}
