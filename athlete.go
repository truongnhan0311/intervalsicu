package intervalsicu

import "context"

// Athlete is a person's intervals.icu profile.
type Athlete struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Email    string   `json:"email,omitempty"`
	Timezone string   `json:"timezone,omitempty"`
	Sex      string   `json:"sex,omitempty"`
	Weight   *float64 `json:"weight,omitempty"`
	// IcuDateFormat and friends are presentation settings; most callers only
	// need ID, which is what every other endpoint keys on.
	IcuDateFormat string `json:"icu_date_format,omitempty"`
}

// Athlete fetches the profile of the configured athlete.
//
// It is the cheapest call in the API, which makes it the natural way to check
// that a credential works before storing it.
func (c *Client) Athlete(ctx context.Context) (*Athlete, error) {
	var athlete Athlete
	if err := c.get(ctx, c.athletePath(""), nil, &athlete); err != nil {
		return nil, err
	}

	return &athlete, nil
}
