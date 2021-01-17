package structs

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"time"

	"github.com/mmcdole/gofeed"
)

type WaitingFeedKind int

const (
	WaitingNone WaitingFeedKind = iota
	WaitingAdd
	WaitingDel
)

const (
	DBPathJobs  = "data/jobs"
	DBPathUsers = "data/users"
)

const (
	ConfigFile = "config.json"
)

type UserInfo struct {
	ID             string
	ChannelId      int64
	WaitingFeedUrl WaitingFeedKind
	Feeds          map[string]bool
}

type JobInfoKey struct {
	User string
	GUID string
}

type JobInfo struct {
	Key JobInfoKey
	RSS gofeed.Item
}

type Config struct {
	Telegram struct {
		Token string
	}
	Feed struct {
		Delay time.Duration
	}
}

var config Config

func GetConfig() Config {
	return config
}

func GetDelay() time.Duration {
	return config.Feed.Delay * time.Second
}

func (k JobInfoKey) Key() []byte {
	return []byte(k.User + ";" + k.GUID)
}

func init() {
	str, err := ioutil.ReadFile(ConfigFile)
	if err != nil {
		log.Panic(err)
	}
	config = Config{}
	json.Unmarshal(str, &config)
}
