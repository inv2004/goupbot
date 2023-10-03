package upwork

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/inv2004/goupbot/internal/upbot/bot"
	"github.com/inv2004/goupbot/internal/upbot/config"
	"github.com/inv2004/goupbot/internal/upbot/model"
	"github.com/mmcdole/gofeed"
	"github.com/recoilme/pudge"
	"github.com/sirupsen/logrus"
)

const TitleSuffix = " | upwork.com"

func HasActiveFeeds(userInfo *model.UserInfo) bool {
	for _, v := range userInfo.Feeds {
		if v.IsActive {
			return true
		}
	}
	return false
}

func repeatURLRequest(bt *bot.BotStruct, fp *gofeed.Parser, url string, times int) (result *gofeed.Feed, err error) {
	ctx, cancel := context.WithTimeout(bt.Ctx, 5*time.Second)
	defer cancel()

	i := 0
	for {
		if result, err = fp.ParseURLWithContext(url, ctx); err != nil {
			i++
			if i == times {
				break
			}
			if err != nil {
				time.Sleep(1 * time.Second)
			}
		} else {
			return result, nil
		}
	}

	return result, err
}

func FetchRss(userId string, fd model.FeedInfo, dryRun bool, bt *bot.BotStruct) (string, error) {
	logrus.WithField("user", userId).Info("fetching for: " + fd.Title)

	fp := gofeed.NewParser()
	feed, err := repeatURLRequest(bt, fp, fd.Url, 3)
	if err != nil {
		return "", err
	}

	title := strings.TrimSuffix(feed.Title, TitleSuffix)

	logrus.WithField("user", userId).Debug("Published: ", feed.Published)
	logrus.WithField("user", userId).Debug("Title: ", title)

	newCounter := 0

	for _, item := range feed.Items {
		key := model.JobInfoKey{User: userId, GUID: item.GUID}

		hasKey, err := pudge.Has(model.DBPathJobs, key.Key())
		if err != nil {
			logrus.Panic(err)
		}
		if !hasKey {
			newCounter += 1
			if !dryRun {
				job := model.JobInfo{}
				job.Key = key
				job.RSS = *item
				logrus.WithField("key", key).Debug("sending job")
				bt.Up2tel <- job
			} else {
				pubVal := model.JobValue{Published: *item.PublishedParsed, Processed: time.Time{}}
				err := pudge.Set(model.DBPathJobs, key.Key(), pubVal)
				if err != nil {
					logrus.Panic(err)
				}
			}
		}
	}

	if dryRun {
		logrus.WithField("counter", newCounter).Info("Drained")
	} else {
		logrus.WithField("counter", newCounter).Info("New")
	}

	return title, nil
}

func FetchUser(userId string, bt *bot.BotStruct) {
	defer bt.Wg.Done()
	defer logrus.WithField("user", userId).Info("fetchUser is going down")

	logrus.WithField("user", userId).Info("fetchUser is started")

	for {
		userInfo := model.UserInfo{}
		err := pudge.Get(model.DBPathUsers, userId, &userInfo)
		if err != nil {
			logrus.Panic(err)
		}

		if !HasActiveFeeds(&userInfo) {
			logrus.WithField("user", userId).Warn("no active feeds found for user")
			return
		}

		pullTimeout := userInfo.Pull
		if pullTimeout == 0 {
			pullTimeout = config.GetDelay()
		}

		select {
		case <-time.After(pullTimeout):
			if !userInfo.Active {
				logrus.WithField("user", userId).Warn("user is not active")
				return
			}

			for _, v := range userInfo.Feeds {
				if !v.IsActive {
					continue
				}
				_, err := FetchRss(userId, v, false, bt)
				if err != nil {
					logrus.Error(err)
					bt.Admin <- err.Error()
				}
			}
		case <-bt.Ctx.Done():
			logrus.WithField("user", userId).Debug("stop fetch")
			return
		}
	}
}

func Start(bt *bot.BotStruct) {
	defer bt.Wg.Done()

	keys, err := pudge.Keys(model.DBPathUsers, nil, 0, 0, true)
	if err != nil {
		logrus.Panic(err)
	}

	for _, userId := range keys {
		bt.Wg.Add(1)
		go FetchUser(string(userId), bt)
	}
}

func AddChannel(userId string, url string, bt *bot.BotStruct) (string, error) {
	userInfo := model.UserInfo{}
	err := pudge.Get(model.DBPathUsers, userId, &userInfo)
	if err != nil {
		log.Panic(err)
	}
	userInfo.WaitingFeedUrl = model.WaitingNone
	title, err := FetchRss(userId, model.FeedInfo{Title: "", Url: url}, true, bt)
	if err != nil {
		return "", err
	}
	userInfo.Feeds = append(userInfo.Feeds, model.FeedInfo{IsActive: true, Title: title, Url: url})
	err = pudge.Set(model.DBPathUsers, userId, userInfo)
	if err != nil {
		logrus.Panic(err)
	}

	if NActiveFeeds(&userInfo) >= 1 {
		bt.Wg.Add(1)
		go FetchUser(userId, bt)
	}

	return title, nil
}

func DelChannel(userId string, idx int, bt *bot.BotStruct) error {
	userInfo := model.UserInfo{}
	err := pudge.Get(model.DBPathUsers, userId, &userInfo)
	if err != nil {
		log.Panic(err)
	}
	userInfo.WaitingFeedUrl = model.WaitingNone

	if !(0 <= idx && idx < len(userInfo.Feeds)) {
		return errors.New("incorrect index to delete")
	}

	userInfo.Feeds = append(userInfo.Feeds[:idx], userInfo.Feeds[idx+1:]...)
	err = pudge.Set(model.DBPathUsers, userId, userInfo)
	if err != nil {
		logrus.Panic(err)
	}

	if err != nil {
		if !errors.Is(err, pudge.ErrKeyNotFound) {
			return err
		}
	}

	if NActiveFeeds(&userInfo) == 1 {
		bt.Wg.Add(1)
		go FetchUser(userId, bt)
	}

	return nil
}

func NActiveFeeds(userInfo *model.UserInfo) (result int) {
	for _, v := range userInfo.Feeds {
		if v.IsActive {
			result += 1
		}
	}
	return
}
