package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

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

func initLastLink() {
	data, err := os.ReadFile(lastLinkPath)
	if err != nil {
		logger.Warnw("No file yet",
			"path", lastLinkPath,
		)
		return
	}
	lastLink = string(data)
	logger.Warnw("Init lastLink",
		"content", lastLink,
	)
}

func main() {
	hookEndpoint = os.Getenv("HOOK_ENDPOINT")
	feedURL = os.Getenv("FEED_URL")
	botUsername = os.Getenv("BOT_USERNAME")
	botAvatarURL = os.Getenv("BOT_AVATAR_URL")
	botMessage = os.Getenv("BOT_MESSAGE")
	lastLinkPath = os.Getenv("LAST_LINK_PATH")
	logger.Infow("Init",
		"feedURL", feedURL,
		"hookEndpoint", hookEndpoint,
	)
	initLastLink()
	go runHealthServer()
	loop(15 * time.Minute)
}

func loop(interval time.Duration) {
	for {
		t0 := time.Now()

		fp := gofeed.NewParser()
		feed, err := fp.ParseURL(feedURL)

		if err != nil {
			logger.Warnw("Failed parsing feed",
				"feedURL", feedURL,
				"err", err,
			)
		} else {
			logger.Infow("Feed fetched",
				"title", feed.Title,
			)
			if len(feed.Items) > 0 {
				link := feed.Items[0].Link
				if link != lastLink {
					logger.Infow("New item",
						"link", link,
					)
					sendNotif(link)
					saveLink(link)
					lastLink = link
				}
			}
		}
		time.Sleep(interval - time.Since(t0))
	}
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

func saveLink(link string) {
	err := os.WriteFile(lastLinkPath, []byte(link), 0644)
	if err != nil {
		logger.Errorw("Error writing file",
			"path", lastLinkPath,
		)
		return
	}
	logger.Infow("Write file",
		"path", lastLinkPath,
		"content", link,
	)

}

func runHealthServer() {
	http.HandleFunc("/healthz", healthzHandler)
	http.ListenAndServe(":8042", nil)
}

func healthzHandler(w http.ResponseWriter, req *http.Request) {
	w.Write([]byte("OK."))
	logger.Infow("Health probe OK.",
		"status", "OK",
		"lastLink", lastLink,
	)
}
