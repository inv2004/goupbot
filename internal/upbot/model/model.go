package model

import (
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

type UserInfo struct {
	ID             string
	ChannelID      int64
	Active         bool
	WaitingFeedUrl WaitingFeedKind
	Feeds          map[string]bool
}

type JobInfoKey struct {
	User string
	GUID string
}

type JobValue struct {
	Published time.Time
	Processed time.Time
}

type JobInfo struct {
	Key JobInfoKey
	RSS gofeed.Item
}
