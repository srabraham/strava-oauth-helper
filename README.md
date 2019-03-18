# strava-oauth-helper

A package that helps in doing local OAuth2 setup for Golang clients for Strava's API.

Much of the code is borrowed from Google's example Go OAuth2 code. It's nice because
it saves your OAuth2 tokens locally for future runs.

This may be of use if you just want to use the Strava APIs to interact with your own
profile, since this just runs a local server to do the OAuth2 callback workflow.

e.g.

```golang
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/srabraham/strava-oauth-helper/stravaauth"

	// Use your choice of repo with Swagger-generated Strava API Golang client code
	strava "github.com/srabraham/swagger-strava-go"
)

func main() {
	flag.Parse()

	scopes := []string{"profile:read_all"}
	oauthCtx, err := stravaauth.GetOAuth2Ctx(context.Background(), strava.ContextOAuth2, scopes)
	if err != nil {
		log.Fatal(err)
	}

	client := strava.NewAPIClient(strava.NewConfiguration())
	athlete, _, err := client.AthletesApi.GetLoggedInAthlete(oauthCtx)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Got athlete = %v", athlete)
}
```

and you'd run that by executing the following, filling in your client ID and client secret from https://www.strava.com/settings/api

```sh
go run myfile.go --strava-clientid=YOUR-CLIENT-ID --strava-secret=YOUR-CLIENT-SECRET
```
