package intervalsicu

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// Common stream names. The set a given activity carries is listed in
// [Activity.StreamTypes].
const (
	StreamWatts     = "watts"
	StreamHeartrate = "heartrate"
	StreamCadence   = "cadence"
	StreamAltitude  = "altitude"
	StreamVelocity  = "velocity_smooth"
	StreamDistance  = "distance"
	StreamTime      = "time"
	StreamTemp      = "temp"
	StreamLatlng    = "latlng"
	StreamTorque    = "torque"
	StreamGrade     = "grade_smooth"
)

// Stream is one channel of per-second samples.
//
// Data holds nil where the device recorded nothing. That is deliberately not
// collapsed to zero: coasting really is zero watts, a dropout is not, and a
// library that conflates them hands every caller a quietly deflated average.
type Stream struct {
	Type string
	Data []*float64
}

// Len returns the number of samples.
func (s Stream) Len() int { return len(s.Data) }

// Floats materialises the channel, substituting missing for absent samples.
//
// Pass math.NaN() to keep gaps detectable, or 0 when a gap should read as no
// effort.
func (s Stream) Floats(missing float64) []float64 {
	out := make([]float64, len(s.Data))
	for i, value := range s.Data {
		if value == nil {
			out[i] = missing

			continue
		}
		out[i] = *value
	}

	return out
}

// Ints materialises the channel as rounded integers, substituting missing for
// absent samples.
//
// A sentinel such as -1 keeps gaps distinguishable from real zeros.
func (s Stream) Ints(missing int) []int {
	out := make([]int, len(s.Data))
	for i, value := range s.Data {
		if value == nil {
			out[i] = missing

			continue
		}
		rounded := int(*value + 0.5)
		if *value < 0 {
			rounded = int(*value - 0.5)
		}
		out[i] = rounded
	}

	return out
}

// Coverage is the share of samples that carry a real value, from 0 to 1.
//
// Worth checking before trusting any average: a ride whose sensor dropped out
// for half its duration still produces a perfectly ordinary looking number.
func (s Stream) Coverage() float64 {
	if len(s.Data) == 0 {
		return 0
	}

	present := 0
	for _, value := range s.Data {
		if value != nil {
			present++
		}
	}

	return float64(present) / float64(len(s.Data))
}

// Streams is the set of channels for one activity, keyed by stream name.
type Streams map[string]Stream

// Get returns a channel and whether it exists.
func (s Streams) Get(name string) (Stream, bool) {
	stream, ok := s[name]

	return stream, ok
}

// Names lists the channels present.
func (s Streams) Names() []string {
	names := make([]string, 0, len(s))
	for name := range s {
		names = append(names, name)
	}

	return names
}

// ActivityStreams fetches per-second samples for one activity.
//
// Pass stream names to limit the response; with none, the server decides what
// to send. Fetching only what you need matters here — a long ride carries tens
// of thousands of points per channel.
func (c *Client) ActivityStreams(ctx context.Context, id string, types ...string) (Streams, error) {
	if id == "" {
		return nil, fmt.Errorf("intervals.icu: empty activity id")
	}

	query := url.Values{}
	if len(types) > 0 {
		query.Set("types", strings.Join(types, ","))
	}

	var raw json.RawMessage
	path := "/activity/" + url.PathEscape(id) + "/streams"
	if err := c.get(ctx, path, query, &raw); err != nil {
		return nil, err
	}

	streams, err := decodeStreams(raw)
	if err != nil {
		return nil, fmt.Errorf("intervals.icu: activity %s streams: %w", id, err)
	}

	return streams, nil
}

// wireStream is the array form of a stream response.
type wireStream struct {
	Type string     `json:"type"`
	Data []*float64 `json:"data"`
}

// decodeStreams reads either response shape the API has been observed to use.
//
// Both an array of {type, data} objects and a plain {name: [...]} map are
// accepted. Guessing one and returning nothing for the other would look
// exactly like an activity that simply has no streams, which is the kind of
// failure nobody notices until the numbers are already wrong.
func decodeStreams(raw json.RawMessage) (Streams, error) {
	out := Streams{}

	var asArray []wireStream
	if err := json.Unmarshal(raw, &asArray); err == nil {
		for _, item := range asArray {
			if item.Type == "" {
				continue
			}
			out[item.Type] = Stream{Type: item.Type, Data: item.Data}
		}

		return out, nil
	}

	var asObject map[string][]*float64
	if err := json.Unmarshal(raw, &asObject); err == nil {
		for name, data := range asObject {
			out[name] = Stream{Type: name, Data: data}
		}

		return out, nil
	}

	return nil, fmt.Errorf("response is neither an array nor an object of streams")
}
