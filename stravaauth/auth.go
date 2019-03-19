package stravaauth

import (
	"context"
	"encoding/gob"
	"errors"
	"flag"
	"fmt"
	"hash/fnv"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

var (
	clientID     = flag.String("strava-clientid", "", "OAuth 2.0 Client ID.  If non-empty, overrides --clientid_file")
	clientIDFile = flag.String("strava-clientid-file", "clientid.dat",
		"Name of a file containing just the project's OAuth 2.0 Client ID.")
	secret     = flag.String("strava-secret", "", "OAuth 2.0 Client Secret.  If non-empty, overrides --secret_file")
	secretFile = flag.String("strava-secret-file", "clientsecret.dat",
		"Name of a file containing just the project's OAuth 2.0 Client Secret.")
	cacheToken = flag.Bool("strava-cachetoken", true, "cache the OAuth 2.0 token")

	tokenFilePrefix = "strava-auth-tok"
)

// GetOAuth2Ctx returns an authenticated Context that can be used to call the Strava API.
//
// e.g. client.AthletesApi.GetLoggedInAthlete(contextReturnedByThisMethod)
//
// The oauth2ContextType should be "strava.ContextOAuth2", using your Swagger-generated "strava" package.
// Having this passed in avoids this stravaauth package from needing to depend on the Swagger-generated
// Strava API code directly.
func GetOAuth2Ctx(parentCtx context.Context, oauth2ContextType fmt.Stringer, scopes []string) (context.Context, error) {
	if !flag.Parsed() {
		return nil, errors.New("Must call Flag.Parse() before GetOAuth2Ctx()")
	}
	if !strings.Contains(oauth2ContextType.String(), "token") {
		return nil, errors.New("You must call GetOAuth2Ctx with oauth2ContextType set to strava.ContextOAuth2")
	}
	config := &oauth2.Config{
		ClientID:     valueOrFileContents(*clientID, *clientIDFile),
		ClientSecret: valueOrFileContents(*secret, *secretFile),
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://www.strava.com/oauth/authorize",
			TokenURL: "https://www.strava.com/oauth/token",
		},
		// Strava expects one string of comma-separated scopes.
		Scopes: []string{strings.Join(scopes, ",")},
	}
	tok := getOAuthToken(parentCtx, config)
	tokSource := config.TokenSource(parentCtx, tok)
	oauthCtx := context.WithValue(parentCtx, oauth2ContextType, tokSource)
	return oauthCtx, nil
}

func osUserCacheDir() string {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		log.Fatalf("Error getting UserCacheDir: %v", err)
	}
	subDir := filepath.Join(cacheDir, "OAuthTokens")
	if err := os.MkdirAll(subDir, 0770); err != nil {
		log.Fatalf("Failed getting or making cache dir: %v", err)
	}
	return subDir
}

func tokenCacheFile(config *oauth2.Config) string {
	hash := fnv.New32a()
	hash.Write([]byte(config.ClientID))
	hash.Write([]byte(config.ClientSecret))
	hash.Write([]byte(strings.Join(config.Scopes, " ")))
	fn := fmt.Sprintf("%s%v", tokenFilePrefix, hash.Sum32())
	return filepath.Join(osUserCacheDir(), url.QueryEscape(fn))
}

func tokenFromFile(file string) (*oauth2.Token, error) {
	if !*cacheToken {
		return nil, errors.New("--cachetoken is false")
	}
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	t := new(oauth2.Token)
	err = gob.NewDecoder(f).Decode(t)
	return t, err
}

func saveToken(file string, token *oauth2.Token) {
	f, err := os.Create(file)
	if err != nil {
		log.Printf("Warning: failed to cache oauth token: %v", err)
		return
	}
	defer f.Close()
	gob.NewEncoder(f).Encode(token)
}

func getOAuthToken(ctx context.Context, config *oauth2.Config) *oauth2.Token {
	cacheFile := tokenCacheFile(config)
	token, err := tokenFromFile(cacheFile)
	if err != nil {
		token = tokenFromWeb(ctx, config)
		saveToken(cacheFile, token)
		log.Printf("Saved new token %#v to %q", token, cacheFile)
	} else {
		log.Printf("Using cached token %#v from %q", token, cacheFile)
	}
	return token
}

func tokenFromWeb(ctx context.Context, config *oauth2.Config) *oauth2.Token {
	ch := make(chan string)
	randState := fmt.Sprintf("st%d", time.Now().UnixNano())
	ts := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/favicon.ico" {
			http.Error(rw, "", 404)
			return
		}
		if req.FormValue("state") != randState {
			log.Printf("State doesn't match: req = %#v", req)
			http.Error(rw, "", 500)
			return
		}
		if code := req.FormValue("code"); code != "" {
			fmt.Fprintf(rw, "<h1>Success</h1>Authorized.")
			rw.(http.Flusher).Flush()
			ch <- code
			return
		}
		log.Printf("no code")
		http.Error(rw, "", 500)
	}))
	defer ts.Close()

	config.RedirectURL = ts.URL
	authURL := config.AuthCodeURL(randState)
	go openURL(authURL)
	log.Printf("Authorize this app at: %s", authURL)
	code := <-ch
	log.Printf("Got code: %s", code)

	token, err := config.Exchange(ctx, code)
	if err != nil {
		log.Fatalf("Token exchange error: %v", err)
	}
	return token
}

func openURL(url string) {
	try := []string{"xdg-open", "google-chrome", "open"}
	for _, bin := range try {
		err := exec.Command(bin, url).Run()
		if err == nil {
			return
		}
	}
	log.Printf("Error opening URL in browser.")
}

func valueOrFileContents(value string, filename string) string {
	if value != "" {
		return value
	}
	slurp, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Fatalf("Error reading %q: %v", filename, err)
	}
	return strings.TrimSpace(string(slurp))
}
