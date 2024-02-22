package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"github.com/mmcdole/gofeed"
	"go.uber.org/zap"
)

var (
	hookEndpoint string
	feedURL      string
	botUsername  string
	botAvatarURL string
	botMessage   string
)

var logger *zap.SugaredLogger

var (
	lastLink     string
	lastLinkPath string
)

func init() {
	l, err := zap.NewProduction()
	if err != nil {
		panic(err)
	}
	logger = l.Sugar()
}

func main() {
	hookEndpoint = os.Getenv("HOOK_ENDPOINT")
	feedURL = os.Getenv("FEED_URL")
	botUsername = os.Getenv("BOT_USERNAME")
	botAvatarURL = os.Getenv("BOT_AVATAR_URL")
	botMessage = os.Getenv("BOT_MESSAGE")
	stateURI := os.Getenv("STATE_URI")

	var state State
	var stateType string
	if strings.HasPrefix(stateURI, "dynamodb://") {
		table, _ := strings.CutPrefix(stateURI, "dynamodb://")
		state = NewDynamoDBStoreWithEnvCreds(table)
		stateType = "DynamoDB table " + table
		defer func() {
			logger.Infow("DynamoDB Consumed",
				"read_units", state.(*DynamoDBState).Consumed,
			)
		}()
	} else {
		state = NewFileState(stateURI)
		stateType = "LocalFile " + stateURI
	}

	previous, _ := state.Get()
	logger.Infow("Previous",
		"link", previous,
		"source", stateType,
	)

	current, _ := getCurrentLinkFromFeed(feedURL)
	logger.Infow("Current",
		"link", current,
		"source", "RSS",
	)

	if previous != current {
		sendNotif(current)
		state.Set(current)
	}
}

func getCurrentLinkFromFeed(feedURL string) (string, error) {
	fp := gofeed.NewParser()
	feed, err := fp.ParseURL(feedURL)

	if err != nil {
		logger.Warnw("Failed parsing feed",
			"feedURL", feedURL,
			"err", err,
		)
		return "", err
	}

	logger.Infow("Feed fetched",
		"title", feed.Title,
	)
	if len(feed.Items) > 0 {
		return feed.Items[0].Link, nil
	}
	return "", nil
}

func sendNotif(link string) {
	message := fmt.Sprintf(botMessage, link)
	jsonBuf, _ := json.Marshal(struct {
		Username  string `json:"username"`
		AvatarURL string `json:"avatar_url"`
		Content   string `json:"content"`
	}{
		botUsername,
		botAvatarURL,
		message,
	})

	req, _ := http.NewRequest("POST", hookEndpoint, bytes.NewBuffer(jsonBuf))
	req.Header.Set("Content-Type", "application/json")
	fmt.Printf("%+v\n", req)
	client := &http.Client{}
	resp, err := client.Do(req)

	if err != nil {
		logger.Errorw("Failed POSTing data",
			"err", err,
		)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 204 {
		body, _ := ioutil.ReadAll(resp.Body)
		logger.Warnw("Return code is not 204",
			"statusCode", resp.StatusCode,
			"body", string(body),
		)
		return
	}
	logger.Infow("Payload sent!",
		"json", string(jsonBuf),
	)
}
