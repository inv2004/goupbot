package upwork

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/inv2004/goupbot/internal/upbot/bot"
	"github.com/inv2004/goupbot/internal/upbot/config"
	"github.com/inv2004/goupbot/internal/upbot/model"
	"github.com/mmcdole/gofeed"
	"github.com/recoilme/pudge"
	"github.com/sirupsen/logrus"
)

func HasActiveFeeds(userInfo *model.UserInfo) bool {
	for _, v := range userInfo.Feeds {
		if v {
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

func FetchRss(userInfo model.UserInfo, url string, dryRun bool, bt *bot.BotStruct) error {
	logrus.WithField("user", userInfo.ID).Info("fetching for")

	fp := gofeed.NewParser()
	feed, err := repeatURLRequest(bt, fp, url, 3)
	if err != nil {
		return err
	}
	logrus.WithField("user", userInfo.ID).Debug("Published: ", feed.Published)
	logrus.WithField("user", userInfo.ID).Debug("Title: ", feed.Published)

	newCounter := 0

	for _, item := range feed.Items {
		key := model.JobInfoKey{User: userInfo.ID, GUID: item.GUID}

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

	return nil
}

func FetchUser(user string, bt *bot.BotStruct) {
	defer bt.Wg.Done()
	defer logrus.WithField("user", user).Info("fetchUser is going down")

	logrus.WithField("user", user).Info("fetchUser is started")

	for {
		select {
		case <-time.After(config.GetDelay()):
			userInfo := model.UserInfo{}
			err := pudge.Get(model.DBPathUsers, user, &userInfo)
			if err != nil {
				logrus.Panic(err)
			}

			if !userInfo.Active {
				logrus.WithField("user", userInfo.ID).Warn("user is not active")
				return
			}

			for url, active := range userInfo.Feeds {
				if !active {
					continue
				}
				err := FetchRss(userInfo, url, false, bt)
				if err != nil {
					logrus.Error(err)
					bt.Admin <- err.Error()
				}
			}

			if !HasActiveFeeds(&userInfo) {
				logrus.WithField("user", userInfo.ID).Warn("no active feeds found for user")
				return
			}
		case <-bt.Ctx.Done():
			logrus.WithField("user", user).Debug("stop fetch")
			return
		case userToCancel := <-bt.Stop2user:
			if user == userToCancel {
				logrus.WithField("user", user).Warn("received cancel")
				return
			}
		}
	}
}

func Start(bt *bot.BotStruct) {
	defer bt.Wg.Done()

	keys, err := pudge.Keys(model.DBPathUsers, nil, 0, 0, true)
	if err != nil {
		logrus.Panic(err)
	}

	for _, user := range keys {
		bt.Wg.Add(1)
		go FetchUser(string(user), bt)
	}
}

func Save(user string, userInfo model.UserInfo) {
	err := pudge.Set(model.DBPathUsers, user, userInfo)
	if err != nil {
		logrus.Panic(err)
	}
}

func AddChannel(user string, url string, bt *bot.BotStruct) error {
	userInfo := model.UserInfo{}
	err := pudge.Get(model.DBPathUsers, user, &userInfo)
	if err != nil {
		log.Panic(err)
	}
	err = FetchRss(userInfo, url, true, bt)
	userInfo.WaitingFeedUrl = model.WaitingNone
	if err == nil {
		userInfo.Feeds[url] = true
	}
	Save(user, userInfo)
	if err != nil {
		return err
	}

	if NActiveFeeds(&userInfo) >= 1 {
		bt.Wg.Add(1)
		go FetchUser(userInfo.ID, bt)
	}

	return nil
}

func DelChannel(user string, url string, bt *bot.BotStruct) error {
	userInfo := model.UserInfo{}
	err := pudge.Get(model.DBPathUsers, user, &userInfo)
	if err != nil {
		log.Panic(err)
	}
	userInfo.WaitingFeedUrl = model.WaitingNone
	userInfo.Feeds[url] = false
	Save(user, userInfo)
	if err != nil {
		if !errors.Is(err, pudge.ErrKeyNotFound) {
			return err
		}
	}

	if NActiveFeeds(&userInfo) == 1 {
		bt.Wg.Add(1)
		go FetchUser(userInfo.ID, bt)
	}

	return nil
}

func NActiveFeeds(userInfo *model.UserInfo) (result int) {
	for _, v := range userInfo.Feeds {
		if v {
			result += 1
		}
	}
	return
}
