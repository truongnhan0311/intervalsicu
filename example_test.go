package intervalsicu_test

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/truongnhan0311/intervalsicu"
)

func Example() {
	client := intervalsicu.New("your-api-key")
	ctx := context.Background()

	athlete, err := client.Athlete(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("signed in as", athlete.Name)

	newest := time.Now()
	oldest := newest.AddDate(0, 0, -30)

	activities, err := client.Activities(ctx, oldest, newest)
	if err != nil {
		log.Fatal(err)
	}

	for _, activity := range activities {
		// Skip estimated power. Strava fills in a number for rides without a
		// meter, and training load built on it is a guess wearing a figure's
		// clothes.
		if !activity.HasRealPower() {
			continue
		}

		streams, err := client.ActivityStreams(ctx, activity.ID, intervalsicu.StreamWatts)
		if err != nil {
			log.Fatal(err)
		}

		watts, ok := streams.Get(intervalsicu.StreamWatts)
		if !ok {
			continue
		}

		fmt.Printf("%s: %d samples, %.0f%% coverage\n",
			activity.Start().Format(time.DateOnly), watts.Len(), watts.Coverage()*100)
	}
}

func ExampleClient_Wellness() {
	client := intervalsicu.New("your-api-key")

	newest := time.Now()
	oldest := newest.AddDate(0, 0, -7)

	records, err := client.Wellness(context.Background(), oldest, newest)
	if err != nil {
		log.Fatal(err)
	}

	for _, record := range records {
		if record.Hrv == nil {
			continue
		}
		fmt.Printf("%s HRV %.1f ms\n", record.ID, *record.Hrv)
	}
}
