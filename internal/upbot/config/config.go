package config

import (
	"encoding/json"
	"log"
	"os"
	"time"
)

type Config struct {
	Telegram struct {
		Token string
		Admin string
	}
	Feed struct {
		Delay time.Duration
	}
}

const (
	ConfigFile = "config.json"
)

var config Config

func GetConfig() Config {
	return config
}

func GetDelay() time.Duration {
	return config.Feed.Delay * time.Second
}

func GetAdmin() string {
	return config.Telegram.Admin
}

func init() {
	str, err := os.ReadFile(ConfigFile)
	if err != nil {
		log.Panic(err)
	}
	config = Config{}
	err = json.Unmarshal(str, &config)
	if err != nil {
		log.Panic(err)
	}
}
