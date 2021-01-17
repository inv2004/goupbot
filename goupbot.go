package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
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

// func fetch() {
// 	bow := surf.NewBrowser()
// 	bow.SetUserAgent(agent.Chrome())
// 	err := bow.Open("https://www.upwork.com/search/jobs/?q=golang&sort=recency")

// 	if err != nil {
// 		log.Panic(err)
// 	}

// 	title := bow.Title()
// 	log.Println("Title: ", title)
// 	if !strings.HasSuffix(title, "Upwork") {
// 		log.Panic("title is wrong")
// 	}

// 	body := bow.Body()
// 	ioutil.WriteFile("1.html", []byte(body), 0644)

// 	scanner := bufio.NewScanner(strings.NewReader(body))
// 	scanner.Split(bufio.ScanLines)

// 	for scanner.Scan() {
// 		parseJobLine(scanner.Text())
// 	}
// }

func fetchRss(userInfo upbot.UserInfo, url string, up2tel chan upbot.JobInfo, dryRun bool) error {
	log.Println("fetching for", userInfo.ID, "url:", url)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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
		key := (upbot.JobInfoKey{userInfo.ID, item.GUID}).Key()

		hasKey, err := pudge.Has(upbot.DBPathJobs, key)
		if err != nil {
			log.Panic(err)
		}
		if !hasKey {
			newCounter += 1
			if !dryRun {
				job := upbot.JobInfo{}
				job.RSS = *item
				up2tel <- job
				log.Println("sending job:", key)
			} else {
				err := pudge.Set(upbot.DBPathJobs, key, time.Time{})
				if err != nil {
					log.Panic(err)
				}
			}
		}

	}

	if dryRun {
		log.Println("drained:", newCounter)
	} else if newCounter == 0 {
		log.Println("New Counter:", newCounter)
	}

	return nil
}

func fetchUser(user string, up2tel chan upbot.JobInfo) {
	for {
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
			err := fetchRss(userInfo, url, up2tel, false)
			if err != nil {
				log.Panic(err)
			}
			time.Sleep(upbot.GetDelay())
		}

		if !hasActiveFeeds {
			log.Print("WARN: no active feeds found for user ", userInfo.ID)
			return
		}
	}
}

func upwork(wg *sync.WaitGroup, up2tel chan upbot.JobInfo) {
	defer wg.Done()

	keys, err := pudge.Keys(upbot.DBPathUsers, nil, 0, 0, true)
	if err != nil {
		log.Panic(err)
	}

	for _, user := range keys {
		go fetchUser(string(user), up2tel)
	}
}

func processMessage(msg *tgbotapi.Message, up2tel chan upbot.JobInfo) (reply string) {
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
/list       - list all feeds
/add        - add feed
/del				- del feed
`
	case "/start":
		userInfo := upbot.UserInfo{}
		userInfo.ID = msg.From.UserName
		userInfo.ChannelId = msg.Chat.ID
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
			err := pudge.Get(upbot.DBPathUsers, user, &userInfo)
			if err != nil {
				log.Panic(err)
			}
			userInfo.WaitingFeedUrl = upbot.WaitingNone
			err = pudge.Set(upbot.DBPathUsers, user, userInfo)
			if err != nil {
				log.Panic(err)
			}
			err = fetchRss(userInfo, url, up2tel, true)
			if err != nil {
				reply = err.Error()
				return
			}
			userInfo.Feeds[url] = true
			err = pudge.Set(upbot.DBPathUsers, user, userInfo)
			if err != nil {
				log.Panic(err)
			}

			go func() {
				time.Sleep(upbot.GetDelay())
				fetchUser(userInfo.ID, up2tel)
			}()

			reply = "Added succesfully. Default pull interval is 1 minute"
		case upbot.WaitingDel:
			url := text
			err := pudge.Get(upbot.DBPathUsers, user, &userInfo)
			if err != nil {
				log.Panic(err)
			}
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

func telegram(wg *sync.WaitGroup, up2tel chan upbot.JobInfo) {
	defer wg.Done()

	bot, err := tgbotapi.NewBotAPI(upbot.GetConfig().Telegram.Token)
	if err != nil {
		log.Panic(err)
	}

	bot.Debug = false

	log.Printf("Authorized on account %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates, err := bot.GetUpdatesChan(u)

	for {
		select {
		case update := <-updates:
			if update.Message == nil { // ignore any non-Message Updates
				continue
			}

			log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)

			reply := processMessage(update.Message, up2tel)

			msg := tgbotapi.NewMessage(update.Message.Chat.ID, reply)
			msg.ReplyToMessageID = update.Message.MessageID

			bot.Send(msg)
		case up := <-up2tel:
			log.Println("recv:", up.RSS.GUID)

			text := up.RSS.Content

			// text, err = html2md(text)
			// if err != nil {
			// 	log.Panic(err)
			// }

			text = strings.ReplaceAll(text, "<br />", "\n")
			text = strings.ReplaceAll(text, "<br>", "\n")
			text = strings.ReplaceAll(text, "&nbsp;", " ")

			msg := tgbotapi.NewMessage(81258084, text)
			msg.ParseMode = tgbotapi.ModeHTML
			msg.DisableWebPagePreview = true
			_, err := bot.Send(msg)
			if err != nil {
				appendMsgToLog(text, err.Error())
				log.Panic(err)
			}
			pudge.Set(upbot.DBPathJobs, up.Key.Key(), time.Now())
		}
	}
}

func main() {
	var wg sync.WaitGroup

	up2tel := make(chan upbot.JobInfo)

	defer pudge.CloseAll()

	wg.Add(2)
	go telegram(&wg, up2tel)
	go upwork(&wg, up2tel)

	// quit := make(chan os.Signal)
	// signal.Notify(quit, os.Interrupt, os.Kill)

	wg.Wait()

	// <-quit
	// log.Println("Shutdown Server ...")
	// if err := pudge.CloseAll(); err != nil {
	// 	log.Println("Pudge Shutdown err:", err)
	// }
}
