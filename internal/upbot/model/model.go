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

type FeedInfo struct {
	IsActive bool
	Title    string
	Url      string
}

type UserInfo struct {
	UserName       string
	ChannelID      int64
	Pull           time.Duration
	Active         bool
	WaitingFeedUrl WaitingFeedKind
	Feeds          []FeedInfo
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
