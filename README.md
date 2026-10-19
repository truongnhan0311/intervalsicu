# intervalsicu

[![Go Reference](https://pkg.go.dev/badge/github.com/truongnhan0311/intervalsicu.svg)](https://pkg.go.dev/github.com/truongnhan0311/intervalsicu)

A Go client for the [intervals.icu](https://intervals.icu) REST API.

intervals.icu aggregates training data from Garmin, Strava, Wahoo, Polar, Oura
and others, so one connection reaches activities, per-second power streams and
daily wellness from every device a person has linked.

```sh
go get github.com/truongnhan0311/intervalsicu
```

## Quick start

```go
client := intervalsicu.New("your-api-key")

athlete, err := client.Athlete(ctx)

activities, err := client.Activities(ctx, oldest, newest)
activity, err := client.Activity(ctx, "i12345678")
streams, err := client.ActivityStreams(ctx, "i12345678",
    intervalsicu.StreamWatts, intervalsicu.StreamHeartrate)

records, err := client.Wellness(ctx, oldest, newest)
record, err := client.WellnessDay(ctx, date)
```

Create an API key under **Settings → Developer** on intervals.icu.

## Three things worth knowing

**The Basic-auth username is the literal string `API_KEY`.** Not your key, not
your email, not your athlete id — the key is the *password*. Getting this
backwards returns `403` with no hint about the cause. The client always sends
it correctly; this note is for when you are reading the wire.

**Athlete id `0` means "whoever owns this credential."** That is the default,
so the common case needs no id at all. Pass a real one with
`WithAthleteID("i123456")` when acting as a coach.

**`device_watts` decides whether power is real.** When a ride has no power
meter, Strava fills in an estimate derived from speed, gradient and weight,
and intervals.icu passes that number straight through. Use
`activity.HasRealPower()` and treat estimated power as absent for anything
feeding training load — an estimate accepted as a measurement quietly poisons
every figure built on it, and nothing downstream can tell afterwards. A
missing flag counts as estimated.

## Design notes

**Absent is not zero.** Optional numbers are pointers. A ride with no power
meter has no average watts; a ride spent coasting really does average zero.
Stream samples are `[]*float64` for the same reason — `Stream.Ints(-1)` and
`Stream.Floats(math.NaN())` materialise them with a sentinel of your choosing,
and `Stream.Coverage()` reports how much of a channel is real, which is worth
checking before trusting any average.

**Unmapped fields survive.** An activity carries well over a hundred fields and
the set grows. Anything not modelled lands in `Activity.Extra` as raw JSON
rather than disappearing.

**Both stream shapes are accepted.** The API has been observed returning both
an array of `{type, data}` objects and a plain `{name: [...]}` map. Guessing
one and returning nothing for the other looks exactly like an activity with no
streams — a failure nobody notices until the numbers are already wrong.

**Errors carry status, not bodies.** `*intervalsicu.Error` exposes the status
code; `IsUnauthorized` and `IsNotFound` cover the two cases worth branching on.
Response bodies stay out of error strings because an error from an auth
endpoint can echo the credential that was just sent, and errors end up in logs.

**intervals.icu computes its own metrics.** `icu_training_load`,
`icu_intensity`, `icu_ctl` and `icu_atl` are exposed and are useful on their
own. If you compute your own training load, do not mix the two — they carry
intervals.icu's thresholds and decay constants, and a chart blending both is
one nobody can reconcile later.

## Status

Endpoints and field names were cross-checked against the
[py-intervalsicu](https://github.com/rday/py-intervalsicu) client. intervals.icu
publishes no formal OpenAPI contract, so treat unfamiliar fields as provisional
and open an issue if you hit a mismatch.

OAuth 2.0 is supported via `WithBearerToken` if you already hold a token.
Registering an OAuth client requires emailing the intervals.icu team; an API
key needs no approval and works immediately.

Not yet covered: writes (wellness updates, activity edits), workouts, events,
calendars and power curves. Contributions welcome.

## Licence

MIT
