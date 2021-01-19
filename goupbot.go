package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	upbot "./upbot"
	md "github.com/JohannesKaufmann/html-to-markdown"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/mmcdole/gofeed"
	"github.com/recoilme/pudge"
	// "github.com/upwork/golang-upwork/api"
	// "github.com/upwork/golang-upwork/api/routers/jobs"
	// "github.com/upwork/golang-upwork/api/routers/jobs/search"
)

func fetchRss(ctx context.Context, userInfo upbot.UserInfo, url string, up2tel chan upbot.JobInfo, dryRun bool) error {
	log.Println("fetching for", userInfo.ID, "url:", url)

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	fp := gofeed.NewParser()
	feed, err := fp.ParseURLWithContext(url, ctx)
	if err != nil {
		return err
	}
	log.Println("Published:", feed.Published)
	log.Println("Title:", feed.Title)

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
				log.Println("sending job:", key)
				up2tel <- job
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
		log.Println("Drained:", newCounter)
	} else {
		log.Println("New Counter:", newCounter)
	}

	return nil
}

func fetchUser(wg *sync.WaitGroup, ctx context.Context, user string, up2tel chan upbot.JobInfo) {
	defer log.Println("fetchUser[", user, "] is going down")
	defer wg.Done()
	for {
		select {
		case <-time.After(upbot.GetDelay()):
			userInfo := upbot.UserInfo{}
			err := pudge.Get(upbot.DBPathUsers, user, &userInfo)
			if err != nil {
				log.Panic(err)
			}

			hasActiveFeeds := false

			for url, active := range userInfo.Feeds {
				if !active {
					continue
				}
				hasActiveFeeds = true
				err := fetchRss(ctx, userInfo, url, up2tel, false)
				if err != nil {
					log.Panic(err)
				}
			}

			if !hasActiveFeeds {
				log.Print("WARN: no active feeds found for user ", userInfo.ID)
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func upwork(wg *sync.WaitGroup, ctx context.Context, up2tel chan upbot.JobInfo) {
	defer wg.Done()

	keys, err := pudge.Keys(upbot.DBPathUsers, nil, 0, 0, true)
	if err != nil {
		log.Panic(err)
	}

	for _, user := range keys {
		wg.Add(1)
		go fetchUser(wg, ctx, string(user), up2tel)
	}
}

func processMessage(wg *sync.WaitGroup, ctx context.Context, msg *tgbotapi.Message, up2tel chan upbot.JobInfo) (reply string) {
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
		userInfo.Feeds = make(map[string]bool)
		err := pudge.Set(upbot.DBPathUsers, user, userInfo)
		if err != nil {
			log.Panic(err)
		}
		reply = "Thank you for subscribing the bot.\n\nPlease add feed channels by /add command or /help for help"
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
			err = fetchRss(ctx, userInfo, url, up2tel, true)
			if err != nil {
				reply = err.Error()
				return
			}
			userInfo.Feeds[url] = true
			err = pudge.Set(upbot.DBPathUsers, user, userInfo)
			if err != nil {
				log.Panic(err)
			}

			if len(userInfo.Feeds) > 0 {
				go func() {
					wg.Add(1)
					fetchUser(wg, ctx, userInfo.ID, up2tel)
				}()
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

func html2md(html string) (string, error) {
	converter := md.NewConverter("", true, nil)

	markdown, err := converter.ConvertString(html)
	if err != nil {
		return "", err
	}
	return markdown, nil

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

func telegram(wg *sync.WaitGroup, ctx context.Context, up2tel chan upbot.JobInfo) {
	defer wg.Done()
	defer log.Println("Telegram is down")

	bot, err := tgbotapi.NewBotAPI(upbot.GetConfig().Telegram.Token)
	if err != nil {
		log.Panic(err)
	}

	bot.Debug = false

	log.Printf("Authorized on account %s", bot.Self.UserName)

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

			reply := processMessage(wg, ctx, update.Message, up2tel)

			err := SendMsgToChannel(bot, update.Message.Chat.ID, reply, update.Message.MessageID)
			if err != nil {
				log.Panic(err)
			}
		case up := <-up2tel:
			log.Println("recv:", up.Key)

			err := SendMsgToUser(bot, up.Key.User, up.RSS.Content)
			if err != nil {
				log.Panic(err)
			}

			log.Println("saving:", up.Key.Key())
			pubVal := upbot.JobValue{Published: *up.RSS.PublishedParsed, Processed: time.Now()}
			ret := pudge.Set(upbot.DBPathJobs, up.Key.Key(), pubVal)
			if ret != nil {
				log.Panic(err)
			}
		case <-ctx.Done():
			return
		}
	}
}

func main() {
	wg := &sync.WaitGroup{}

	up2tel := make(chan upbot.JobInfo)

	defer func() {
		err := pudge.CloseAll()
		if err != nil {
			log.Panic(err)
		}
		log.Println("db closed")
	}()

	ctx, cancel := context.WithCancel(context.Background())

	wg.Add(2)
	go telegram(wg, ctx, up2tel)
	go upwork(wg, ctx, up2tel)

	quit := make(chan os.Signal)
	signal.Notify(quit, os.Interrupt, os.Kill)

	<-quit
	log.Println("signal to shutdown ...")
	cancel()
	wg.Wait()
}
