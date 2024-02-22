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

	if len(stateURI) == 0 {
		logger.Fatalw("STATE_URI is not specified")
	}

	var state State
	var stateType string
	if strings.HasPrefix(stateURI, "dynamodb://") {
		var err error
		table, _ := strings.CutPrefix(stateURI, "dynamodb://")
		state, err = NewDynamoDBStoreWithEnvCreds(table)
		if err != nil {
			logger.Fatalw(err.Error())
		}
		defer func() {
			logger.Infow("DynamoDB Consumed",
				"read_units", state.(*DynamoDBState).Consumed,
			)
		}()
		stateType = "DynamoDB table " + table
	} else {
		var err error
		state, err = NewFileState(stateURI)
		if err != nil {
			logger.Fatalw(err.Error())
		}
		stateType = "LocalFile " + stateURI
	}

	previous, err := state.Get()
	if err != nil {
		logger.Fatalw(err.Error())
	}
	logger.Infow("Previous",
		"link", previous,
		"source", stateType,
	)

	current, err := getCurrentLinkFromFeed(feedURL)
	if err != nil {
		logger.Fatalw(err.Error())
	}
	logger.Infow("Current",
		"link", current,
		"source", "RSS",
	)

	if previous != current {
		if err := sendNotif(current); err != nil {
			logger.Fatalw(err.Error())
		}
		if err := state.Set(current); err != nil {
			logger.Fatalw(err.Error())
		}
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

func sendNotif(link string) error {
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
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 204 {
		body, err := ioutil.ReadAll(resp.Body)
		logger.Warnw("Return code is not 204",
			"statusCode", resp.StatusCode,
			"body", string(body),
		)
		return err
	}
	logger.Infow("Payload sent!",
		"json", string(jsonBuf),
	)
	return nil
}
