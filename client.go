// Package intervalsicu is a Go client for the intervals.icu REST API.
//
// intervals.icu aggregates training data from Garmin, Strava, Wahoo, Polar,
// Oura and others, so a single connection reaches activities, power streams
// and daily wellness from every device a person has linked.
//
// # Authentication
//
// The API uses HTTP Basic authentication. The username is the literal string
// "API_KEY" — not your key, not your email, not your athlete id — and the
// password is the key you create under Settings → Developer on intervals.icu.
// Getting this backwards returns 403 with no hint about the cause, so the
// client always sends it correctly:
//
//	client := intervalsicu.New("your-api-key")
//	athlete, err := client.Athlete(ctx)
//
// OAuth 2.0 exists for multi-user applications, but registering a client
// requires emailing the intervals.icu team. If you already hold an OAuth
// access token, pass it with [WithBearerToken].
//
// # Athlete id
//
// Every athlete-scoped path takes an id. The special value "0" means "whoever
// owns this credential", which is what the client uses by default. Set a real
// id with [WithAthleteID] when acting as a coach.
package intervalsicu

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultBaseURL is the production API root.
const DefaultBaseURL = "https://intervals.icu/api/v1"

// basicUser is the fixed Basic-auth username required by intervals.icu.
const basicUser = "API_KEY"

// SelfAthlete addresses the athlete that owns the current credential.
const SelfAthlete = "0"

// maxBody caps how much of a response is read into memory.
//
// A six-hour ride holds roughly 21,600 points per stream, so a few megabytes
// of JSON. 64 MiB leaves ample headroom while still bounding a broken or
// hostile response.
const maxBody = 64 << 20

// Client talks to the intervals.icu API.
//
// A Client is safe for concurrent use by multiple goroutines.
type Client struct {
	httpClient *http.Client
	baseURL    string
	athleteID  string
	userAgent  string

	apiKey      string
	bearerToken string
}

// Option configures a [Client].
type Option func(*Client)

// WithAthleteID sets the athlete whose data is requested.
//
// Defaults to [SelfAthlete], which resolves to the credential's own athlete.
// Ids look like "i123456".
func WithAthleteID(id string) Option {
	return func(c *Client) {
		if id != "" {
			c.athleteID = id
		}
	}
}

// WithHTTPClient supplies the HTTP client used for every request.
//
// Use it to set a custom transport, proxy or timeout. The default client has a
// 60 second timeout; http.DefaultClient has none, and an API call that never
// returns will hold a goroutine and whatever it is holding open for good.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		if httpClient != nil {
			c.httpClient = httpClient
		}
	}
}

// WithBaseURL overrides the API root, for tests or a staging deployment.
func WithBaseURL(baseURL string) Option {
	return func(c *Client) {
		if baseURL != "" {
			c.baseURL = strings.TrimSuffix(baseURL, "/")
		}
	}
}

// WithUserAgent sets the User-Agent header.
//
// Identifying your application is good manners against a small service and
// makes your traffic recognisable if you ever need to ask about it.
func WithUserAgent(userAgent string) Option {
	return func(c *Client) {
		if userAgent != "" {
			c.userAgent = userAgent
		}
	}
}

// WithBearerToken authenticates with an OAuth 2.0 access token instead of an
// API key.
//
// Takes precedence over the key passed to [New], so New("") is fine when only
// a token is available.
func WithBearerToken(token string) Option {
	return func(c *Client) {
		c.bearerToken = token
	}
}

// New returns a Client authenticating with the given API key.
//
// Create a key under Settings → Developer on intervals.icu. Pass "" together
// with [WithBearerToken] to use OAuth instead.
func New(apiKey string, opts ...Option) *Client {
	client := &Client{
		httpClient: &http.Client{Timeout: 60 * time.Second},
		baseURL:    DefaultBaseURL,
		athleteID:  SelfAthlete,
		userAgent:  "intervalsicu-go",
		apiKey:     apiKey,
	}

	for _, opt := range opts {
		opt(client)
	}

	return client
}

// AthleteID returns the athlete id the client addresses.
func (c *Client) AthleteID() string { return c.athleteID }

// Error is an unsuccessful API response.
//
// It deliberately carries no response body: an error from an authentication
// endpoint can repeat the credential that was just sent, and errors tend to
// end up in logs.
type Error struct {
	StatusCode int
	Method     string
	Path       string
}

func (e *Error) Error() string {
	return fmt.Sprintf("intervals.icu: %s %s returned %d %s",
		e.Method, e.Path, e.StatusCode, http.StatusText(e.StatusCode))
}

// IsUnauthorized reports whether err is a 401 or 403, which for this API means
// a missing, wrong or revoked credential.
func IsUnauthorized(err error) bool {
	var apiErr *Error
	if !asError(err, &apiErr) {
		return false
	}

	return apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden
}

// IsNotFound reports whether err is a 404.
func IsNotFound(err error) bool {
	var apiErr *Error
	if !asError(err, &apiErr) {
		return false
	}

	return apiErr.StatusCode == http.StatusNotFound
}

// get performs a GET and decodes the JSON body into dest.
func (c *Client) get(ctx context.Context, path string, query url.Values, dest any) error {
	endpoint := c.baseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("intervals.icu: build request for %s: %w", path, err)
	}

	c.authenticate(request)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", c.userAgent)

	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("intervals.icu: GET %s: %w", path, err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		// Drain a little so the connection can be reused, then discard it.
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))

		return &Error{StatusCode: response.StatusCode, Method: http.MethodGet, Path: path}
	}

	if dest == nil {
		return nil
	}

	if err := json.NewDecoder(io.LimitReader(response.Body, maxBody)).Decode(dest); err != nil {
		return fmt.Errorf("intervals.icu: decode %s: %w", path, err)
	}

	return nil
}

// authenticate attaches the credential, preferring an OAuth token when set.
func (c *Client) authenticate(request *http.Request) {
	if c.bearerToken != "" {
		request.Header.Set("Authorization", "Bearer "+c.bearerToken)

		return
	}

	// The username is the fixed string "API_KEY"; the key is the password.
	credential := base64.StdEncoding.EncodeToString([]byte(basicUser + ":" + c.apiKey))
	request.Header.Set("Authorization", "Basic "+credential)
}

// athletePath builds a path under the configured athlete.
func (c *Client) athletePath(suffix string) string {
	return "/athlete/" + url.PathEscape(c.athleteID) + suffix
}

// dateRange renders an oldest/newest query, the filter every list endpoint takes.
func dateRange(oldest, newest time.Time) url.Values {
	query := url.Values{}
	query.Set("oldest", oldest.Format(time.DateOnly))
	query.Set("newest", newest.Format(time.DateOnly))

	return query
}
