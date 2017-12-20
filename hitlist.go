package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"path/filepath"

	"github.com/chimeracoder/anaconda"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	sheets "google.golang.org/api/sheets/v4"
)

var (
	// Sheets flags.
	clientSecretFilePathFlag = flag.String("client_secret_file", "./client_secret.json", "the path of of the Sheets client secret file")
	spreadsheetIDFlag        = flag.String("sheet_id", "", "the id of the spreadsheet to read")
	sheetNameFlag            = flag.String("sheet_name", "Sheet1", "the name of the sheet from which to read")
	readRangeFlag            = flag.String("read_range", "", "the range to read from the sheet (e.g. 'A2:E')")
	// Twitter flags.
	consumerKeyFlag    = flag.String("twitter_consumer_key", "", "the consumer key for the Twitter account")
	consumerSecretFlag = flag.String("twitter_consumer_secret", "", "the consumer secret for the Twitter account")
	accessTokenFlag    = flag.String("twitter_access_token", "", "the access token for the Twitter account")
	accessSecretFlag   = flag.String("twitter_access_secret", "", "the access token secret for the Twitter account")
)

type sheetsConfig struct {
	secretPath, id, name, cellRange string
}

type twitterConfig struct {
	consumerKey, consumerSecret string
	accessToken, accessSecret   string
}

// This code is inspired by the guide here:
// https://developers.google.com/sheets/api/quickstart/go

func main() {
	flag.Parse()

	sc := &sheetsConfig{
		secretPath: *clientSecretFilePathFlag,
		id:         *spreadsheetIDFlag,
		name:       *sheetNameFlag,
		cellRange:  *readRangeFlag,
	}

	tc := &twitterConfig{
		consumerKey:    *consumerKeyFlag,
		consumerSecret: *consumerSecretFlag,
		accessToken:    *accessTokenFlag,
		accessSecret:   *accessSecretFlag,
	}

	if err := doMain(sc, tc); err != nil {
		log.Fatal(err)
	}
}

const permScope = "https://www.googleapis.com/auth/spreadsheets.readonly"

func doMain(sc *sheetsConfig, tc *twitterConfig) error {
	ctx := context.Background()
	secretContent, err := ioutil.ReadFile(sc.secretPath)
	if err != nil {
		return fmt.Errorf("failed to read client secret file: %v", err)
	}

	config, err := google.ConfigFromJSON(secretContent, permScope)
	if err != nil {
		return fmt.Errorf("failed to create config from secret file at %q: %v", sc.secretPath, err)
	}

	client, err := getClient(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to get client for Sheets: %v", err)
	}

	srv, err := sheets.New(client)
	if err != nil {
		return fmt.Errorf("failed to retrieve client for Sheets: %v", err)
	}

	r := fmt.Sprintf("%s!%s", sc.name, sc.cellRange)
	resp, err := srv.Spreadsheets.Values.Get(sc.id, r).Do()
	if err != nil {
		return fmt.Errorf("failed to read sheet with id=%q and range=%q: %v", sc.id, r, err)
	}

	if len(resp.Values) < 1 {
		return errors.New("no data found from spreadsheet")
	}

	anaconda.SetConsumerKey(tc.consumerKey)
	anaconda.SetConsumerKey(tc.consumerSecret)
	api := anaconda.NewTwitterApi(tc.accessToken, tc.accessSecret)

	if err := tweet(api, resp.Values); err != nil {
		return fmt.Errorf("failed to tweet: %v", err)
	}

	if err := markComplete(); err != nil {
		return fmt.Errorf("failed to mark Tweeted data as complete: %v", err)
	}

	return nil
}

func getClient(ctx context.Context, config *oauth2.Config) (*http.Client, error) {
	cacheFile, err := createCacheFile()
	if err != nil {
		return nil, fmt.Errorf("unable to get path to cached credential file: %v", err)
	}

	tok, err := tokenFromFile(cacheFile)
	if err != nil {
		// The token DNE or is invalid, so fetch and cache a new one.
		tok, err = getTokenFromWeb(config)
		if err != nil {
			return nil, fmt.Errorf("failed to get token from web: %v", err)
		}
		saveToken(cacheFile, tok)
	}

	return config.Client(ctx, tok), nil
}

func createCacheFile() (string, error) {
	usr, err := user.Current()
	if err != nil {
		return "", err
	}
	tokenCacheDir := filepath.Join(usr.HomeDir, ".credentials")
	if err := os.MkdirAll(tokenCacheDir, 0700); err != nil {
		return "", err
	}
	return filepath.Join(tokenCacheDir, url.QueryEscape("sheets-to-tweets")), nil
}

func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	t := &oauth2.Token{}
	return t, json.NewDecoder(f).Decode(t)
}

func getTokenFromWeb(config *oauth2.Config) (*oauth2.Token, error) {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	log.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	var code string
	if _, err := fmt.Scan(&code); err != nil {
		return nil, fmt.Errorf("Unable to read authorization code %v", err)
	}

	tok, err := config.Exchange(oauth2.NoContext, code)
	if err != nil {
		return nil, fmt.Errorf("Unable to retrieve token from web %v", err)
	}
	return tok, nil
}

func saveToken(file string, token *oauth2.Token) error {
	log.Printf("Saving credential file to: %s\n", file)
	f, err := os.OpenFile(file, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(token)
}

const maxTweetSize = 280 // wowee!

func tweet(api *anaconda.TwitterApi, data interface{}) error {
	log.Printf("would have tweeted data: %v", data)
	status := fmt.Sprintf("some cool data: %v", data)
	if len(status) > maxTweetSize {
		status = status[:maxTweetSize]
	}

	if _, err := api.PostTweet(status, url.Values{}); err != nil {
		return err
	}

	return nil
}

func markComplete() error {
	return nil
}
