package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	upbot "./upbot"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/mmcdole/gofeed"
	"github.com/recoilme/pudge"
	log "github.com/sirupsen/logrus"
	// "github.com/upwork/golang-upwork/api"
	// "github.com/upwork/golang-upwork/api/routers/jobs"
	// "github.com/upwork/golang-upwork/api/routers/jobs/search"
)

type BotStruct struct {
	wg        *sync.WaitGroup
	ctx       context.Context
	up2tel    chan upbot.JobInfo
	stop2user chan string
}

func fetchRss(userInfo upbot.UserInfo, url string, dryRun bool, bt BotStruct) error {
	log.WithField("user", userInfo.ID).Info("fetching for")

	ctx, cancel := context.WithTimeout(bt.ctx, 10*time.Second)
	defer cancel()

	fp := gofeed.NewParser()
	feed, err := fp.ParseURLWithContext(url, ctx)
	if err != nil {
		return err
	}
	log.WithField("user", userInfo.ID).Debug("Published: ", feed.Published)
	log.WithField("user", userInfo.ID).Debug("Title: ", feed.Published)

	newCounter := 0

	for _, item := range feed.Items {
		key := upbot.JobInfoKey{User: userInfo.ID, GUID: item.GUID}

		hasKey, err := pudge.Has(upbot.DBPathJobs, key.Key())
		if err != nil {
			log.Panic(err)
		}
		if !hasKey {
			newCounter += 1
			if !dryRun {
				job := upbot.JobInfo{}
				job.Key = key
				job.RSS = *item
				log.WithField("key", key).Debug("sending job")
				bt.up2tel <- job
			} else {
				pubVal := upbot.JobValue{Published: *item.PublishedParsed, Processed: time.Time{}}
				err := pudge.Set(upbot.DBPathJobs, key.Key(), pubVal)
				if err != nil {
					log.Panic(err)
				}
			}
		}
	}

	if dryRun {
		log.WithField("counter", newCounter).Info("Drained")
	} else {
		log.WithField("counter", newCounter).Info("New")
	}

	return nil
}

func fetchUser(user string, bt BotStruct) {
	defer bt.wg.Done()
	defer log.Println("fetchUser[", user, "] is going down")

	for {
		select {
		case <-time.After(upbot.GetDelay()):
			userInfo := upbot.UserInfo{}
			err := pudge.Get(upbot.DBPathUsers, user, &userInfo)
			if err != nil {
				log.Panic(err)
			}

			if !userInfo.Active {
				log.WithField("user", userInfo.ID).Warn("user is not active")
				return
			}

			hasActiveFeeds := false
			for url, active := range userInfo.Feeds {
				if !active {
					continue
				}
				hasActiveFeeds = true
				err := fetchRss(userInfo, url, false, bt)
				if err != nil {
					log.Panic(err)
				}
			}

			if !hasActiveFeeds {
				log.WithField("user", userInfo.ID).Warn("no active feeds found for user")
				return
			}
		case <-bt.ctx.Done():
			return
		case userToCancel := <-bt.stop2user:
			if user == userToCancel {
				log.WithField("user", user).Warn("received cancel")
				return
			}
		}
	}
}

func upwork(bt BotStruct) {
	defer bt.wg.Done()

	keys, err := pudge.Keys(upbot.DBPathUsers, nil, 0, 0, true)
	if err != nil {
		log.Panic(err)
	}

	for _, user := range keys {
		bt.wg.Add(1)
		go fetchUser(string(user), bt)
	}
}

func processMessage(msg *tgbotapi.Message, bt BotStruct) (reply string) {
	user := msg.From.UserName
	text := msg.Text
	// words := strings.Fields(text)
	// if len(words) == 0 {
	// 	log.Println("WARN: wrong command")
	// 	reply = "wrong command"
	// 	return
	// }

	// cmd := words[0]

	switch text {
	case "/help":
		reply = `
/help       - this help
/start      - start publishing
/stop       - stop
/ping       - pong
/list       - list all your feeds
/add        - add feed
/del				- del feed
`
	case "/start":
		userInfo := upbot.UserInfo{}
		userInfo.ID = msg.From.UserName
		userInfo.ChannelID = msg.Chat.ID
		userInfo.Active = true
		userInfo.Feeds = make(map[string]bool)
		err := pudge.Set(upbot.DBPathUsers, user, userInfo)
		if err != nil {
			log.Panic(err)
		}
		reply = "Thank you for subscribing the bot.\n\nPlease add feed channels by /add command or /help for help"
	case "/stop":
		userInfo := upbot.UserInfo{}
		err := pudge.Get(upbot.DBPathUsers, msg.From.UserName, userInfo)
		if err != nil {
			if errors.Is(err, pudge.ErrKeyNotFound) {
				reply = "nothing to stop"
				return
			}
		}
		if len(userInfo.Feeds) == 0 {
			reply = "no feeds to stop"
			return
		}
		userInfo.Active = false
		err = pudge.Set(upbot.DBPathUsers, user, userInfo)
		if err != nil {
			log.Panic(err)
		}
		bt.stop2user <- msg.From.UserName
		reply = "Your feed stopped, Type /start to continue"
	case "/ping":
		reply = "pong"
	case "/add":
		userInfo := upbot.UserInfo{}
		err := pudge.Get(upbot.DBPathUsers, user, &userInfo)
		if err != nil {
			if errors.Is(err, pudge.ErrKeyNotFound) {
				reply = "Type /start first"
				return
			} else {
				log.Panic(err)
			}
		}
		userInfo.WaitingFeedUrl = upbot.WaitingAdd
		err = pudge.Set(upbot.DBPathUsers, user, userInfo)
		if err != nil {
			log.Panic(err)
		}
		reply = "Paste rss url to add here:"
	case "/del":
		userInfo := upbot.UserInfo{}
		err := pudge.Get(upbot.DBPathUsers, user, &userInfo)
		if err != nil {
			if errors.Is(err, pudge.ErrKeyNotFound) {
				reply = "Type /start first"
				return
			} else {
				log.Panic(err)
			}
		}
		userInfo.WaitingFeedUrl = upbot.WaitingDel
		err = pudge.Set(upbot.DBPathUsers, user, userInfo)
		if err != nil {
			log.Panic(err)
		}
		reply = "Paste rss url to delete here:"
	case "/list":
		userInfo := upbot.UserInfo{}
		err := pudge.Get(upbot.DBPathUsers, user, &userInfo)
		if err != nil {
			if errors.Is(err, pudge.ErrKeyNotFound) {
				reply = "Type /start first"
				return
			} else {
				log.Panic(err)
			}
		}

		i := 0
		for k, v := range userInfo.Feeds {
			if !v {
				continue
			}
			reply += fmt.Sprintf("%d) %s\n", i+1, k)
			i += 1
		}
		if i == 0 {
			reply = "Empty"
		}
	default:
		userInfo := upbot.UserInfo{}
		err := pudge.Get(upbot.DBPathUsers, user, &userInfo)
		if err != nil {
			log.Panic(err)
		}
		switch userInfo.WaitingFeedUrl {
		case upbot.WaitingAdd:
			url := text
			userInfo.WaitingFeedUrl = upbot.WaitingNone
			err = pudge.Set(upbot.DBPathUsers, user, userInfo)
			if err != nil {
				log.Panic(err)
			}
			err = fetchRss(userInfo, url, true, bt)
			if err != nil {
				reply = err.Error()
				return
			}
			userInfo.Feeds[url] = true
			err = pudge.Set(upbot.DBPathUsers, user, userInfo)
			if err != nil {
				log.Panic(err)
			}

			if len(userInfo.Feeds) == 1 {
				bt.wg.Add(1)
				go fetchUser(userInfo.ID, bt)
			}

			reply = "Added succesfully. Default pull interval is 1 minute"
		case upbot.WaitingDel:
			url := text
			userInfo.WaitingFeedUrl = upbot.WaitingNone
			_, ok := userInfo.Feeds[url]
			if ok {
				userInfo.Feeds[url] = false
			}
			err = pudge.Set(upbot.DBPathUsers, user, userInfo)
			if err != nil {
				log.Panic(err)
			}
			reply = "Rss removed"
		default:
			reply = "Unknown command"
		}
	}
	return
}

func appendMsgToLog(text string, errText string) {
	f, err := os.OpenFile("err_msgs.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Panic(err)
	}
	defer f.Close()
	if _, err := f.WriteString(text + "\n^^^ " + errText + "\n"); err != nil {
		log.Panic(err)
	}
}

func SendMsgToChannel(bot *tgbotapi.BotAPI, channel int64, text string, replyTo int) (err error) {
	text = strings.ReplaceAll(text, "<br />", "\n")
	text = strings.ReplaceAll(text, "<br/>", "\n")
	text = strings.ReplaceAll(text, "<br>", "\n")
	text = strings.ReplaceAll(text, "&nbsp;", " ")

	if len(text) > 4096 {
		text = text[:4092] + " ..."
	}

	msg := tgbotapi.NewMessage(channel, text)
	msg.ParseMode = tgbotapi.ModeHTML
	msg.DisableWebPagePreview = true

	if replyTo > 0 {
		msg.ReplyToMessageID = replyTo
	}

	_, err = bot.Send(msg)
	if err != nil {
		appendMsgToLog(text, err.Error())
	}
	return
}

func SendMsgToUser(bot *tgbotapi.BotAPI, user string, text string) error {
	userInfo := upbot.UserInfo{}
	err := pudge.Get(upbot.DBPathUsers, user, &userInfo)
	if err != nil {
		return err
	}

	err = SendMsgToChannel(bot, userInfo.ChannelID, text, 0)
	return err
}

func telegram(bt BotStruct) {
	defer bt.wg.Done()
	defer log.Warn("Telegram is down")

	bot, err := tgbotapi.NewBotAPI(upbot.GetConfig().Telegram.Token)
	if err != nil {
		log.Panic(err)
	}

	bot.Debug = false

	log.WithField("bot", bot.Self.UserName).Info("Authorized on account")

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates, err := bot.GetUpdatesChan(u)
	if err != nil {
		log.Panic(err)
	}

	for {
		select {
		case update := <-updates:
			if update.Message == nil { // ignore any non-Message Updates
				continue
			}

			log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)

			reply := processMessage(update.Message, bt)

			err := SendMsgToChannel(bot, update.Message.Chat.ID, reply, update.Message.MessageID)
			if err != nil {
				log.Panic(err)
			}
		case up := <-bt.up2tel:
			log.WithField("key", up.Key).Debug("recv")

			err := SendMsgToUser(bot, up.Key.User, up.RSS.Content)
			if err != nil {
				log.Panic(err)
			}

			log.WithField("key", up.Key).Debug("saving")
			pubVal := upbot.JobValue{Published: *up.RSS.PublishedParsed, Processed: time.Now()}
			ret := pudge.Set(upbot.DBPathJobs, up.Key.Key(), pubVal)
			if ret != nil {
				log.Panic(err)
			}
		case <-bt.ctx.Done():
			return
		}
	}
}

func main() {
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
	})

	ctx, cancel := context.WithCancel(context.Background())

	bt := BotStruct{
		wg:        &sync.WaitGroup{},
		ctx:       ctx,
		up2tel:    make(chan upbot.JobInfo),
		stop2user: make(chan string),
	}

	defer func() {
		err := pudge.CloseAll()
		if err != nil {
			log.Panic(err)
		}
		log.Info("db closed")
	}()

	bt.wg.Add(2)
	go telegram(bt)
	go upwork(bt)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	<-quit
	log.Warn("signal to shutdown ...")
	cancel()
	bt.wg.Wait()
}
