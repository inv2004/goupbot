package telegram

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/inv2004/goupbot/internal/upbot/bot"
	"github.com/inv2004/goupbot/internal/upbot/config"
	"github.com/inv2004/goupbot/internal/upbot/model"
	"github.com/inv2004/goupbot/internal/upbot/upwork"
	"github.com/recoilme/pudge"
	"github.com/sirupsen/logrus"
)

func SendMsgToChannel(bot *tgbotapi.BotAPI, channel int64, text string, replyTo int) (err error) {
	text = strings.ReplaceAll(text, "<br />", "\n")
	text = strings.ReplaceAll(text, "<br/>", "\n")
	text = strings.ReplaceAll(text, "<br>", "\n")
	text = strings.ReplaceAll(text, "&nbsp;", " ")

	if len(text) > 4096 {
		text = text[:4092]
		idx := strings.LastIndex(text, "\n")
		if idx > 0 {
			text = text[:idx-1] + " ..."
		}
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

func appendMsgToLog(text string, errText string) {
	f, err := os.OpenFile("err_msgs.logrus", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		logrus.Panic(err)
	}
	defer f.Close()
	if _, err := f.WriteString(text + "\n^^^ " + errText + "\n"); err != nil {
		logrus.Panic(err)
	}
}

func SendMsgToUser(bot *tgbotapi.BotAPI, user string, text string) error {
	userInfo := model.UserInfo{}
	err := pudge.Get(model.DBPathUsers, user, &userInfo)
	if err != nil {
		return err
	}

	err = SendMsgToChannel(bot, userInfo.ChannelID, text, 0)
	return err
}

func processMessage(msg *tgbotapi.Message, bt *bot.BotStruct) (reply string) {
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
		userInfo := model.UserInfo{}
		err := pudge.Get(model.DBPathUsers, user, &userInfo)
		if err != nil {
			if errors.Is(err, pudge.ErrKeyNotFound) {
				userInfo.ID = user
				userInfo.ChannelID = msg.Chat.ID
				userInfo.Feeds = make(map[string]bool)
			}
		}

		if userInfo.Active {
			reply = "Your user is active already"
			return
		}

		userInfo.Active = true
		err = pudge.Set(model.DBPathUsers, user, userInfo)
		if err != nil {
			logrus.Panic(err)
		}
		feedInfo := ""
		if upwork.HasActiveFeeds(&userInfo) {
			feedInfo = "You have some channels already, check with /list\n\n"
			bt.Wg.Add(1)
			go upwork.FetchUser(userInfo.ID, bt)
		}

		reply = "Thank you for subscribing the bot.\n\n" + feedInfo + "Please add feed channels by /add command or /help for help"
	case "/stop":
		userInfo := model.UserInfo{}
		err := pudge.Get(model.DBPathUsers, user, &userInfo)
		if err != nil {
			if errors.Is(err, pudge.ErrKeyNotFound) {
				reply = "nothing to stop"
				return
			}
			logrus.Panic(err)
		}
		if !userInfo.Active {
			reply = "stopped already"
			return
		}
		if len(userInfo.Feeds) == 0 {
			reply = "no feeds to stop"
			return
		}
		userInfo.Active = false
		logrus.WithField("user", user).WithField("userInfo", userInfo).Debug("Store")
		err = pudge.Set(model.DBPathUsers, user, &userInfo)
		if err != nil {
			logrus.Panic(err)
		}
		bt.Stop2user <- msg.From.UserName
		reply = "Your user and feeds are suspended, Type /start to resume"
	case "/ping":
		reply = "pong"
	case "/add":
		userInfo := model.UserInfo{}
		err := pudge.Get(model.DBPathUsers, user, &userInfo)
		if err != nil {
			if errors.Is(err, pudge.ErrKeyNotFound) {
				reply = "Type /start first"
				return
			} else {
				logrus.Panic(err)
			}
		}
		if userInfo.Active == false {
			reply = "Type /start to resume"
			return
		}
		userInfo.WaitingFeedUrl = model.WaitingAdd
		err = pudge.Set(model.DBPathUsers, user, userInfo)
		if err != nil {
			logrus.Panic(err)
		}
		reply = "Paste rss url to add here:"
	case "/del":
		userInfo := model.UserInfo{}
		err := pudge.Get(model.DBPathUsers, user, &userInfo)
		if err != nil {
			if errors.Is(err, pudge.ErrKeyNotFound) {
				reply = "Type /start first"
				return
			} else {
				logrus.Panic(err)
			}
		}
		userInfo.WaitingFeedUrl = model.WaitingDel
		err = pudge.Set(model.DBPathUsers, user, userInfo)
		if err != nil {
			logrus.Panic(err)
		}
		reply = "Paste rss url to delete here:"
	case "/list":
		userInfo := model.UserInfo{}
		err := pudge.Get(model.DBPathUsers, user, &userInfo)
		if err != nil {
			if errors.Is(err, pudge.ErrKeyNotFound) {
				reply = "Type /start first"
				return
			} else {
				logrus.Panic(err)
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
		userInfo := model.UserInfo{}
		err := pudge.Get(model.DBPathUsers, user, &userInfo)
		if err != nil {
			logrus.Panic(err)
		}
		switch userInfo.WaitingFeedUrl {
		case model.WaitingAdd:
			err = upwork.AddChannel(user, text, bt)
			if err != nil {
				reply = err.Error()
				return
			}

			reply = "Added succesfully. Default pull interval is 1 minute"
		case model.WaitingDel:
			err = upwork.DelChannel(user, text, bt)
			if err != nil {
				reply = err.Error()
				return
			}

			reply = "feed removed"
		default:
			reply = "Unknown command"
		}
	}
	return
}

func Start(bt *bot.BotStruct) {
	defer bt.Wg.Done()
	defer logrus.Warn("Telegram is down")

	bot, err := tgbotapi.NewBotAPI(config.GetConfig().Telegram.Token)
	if err != nil {
		logrus.Panic(err)
	}

	bot.Debug = false

	logrus.WithField("bot", bot.Self.UserName).Info("Authorized on account")

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates, err := bot.GetUpdatesChan(u)
	if err != nil {
		logrus.Panic(err)
	}

	for {
		select {
		case update := <-updates:
			if update.Message == nil { // ignore any non-Message Updates
				continue
			}

			logrus.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)

			reply := processMessage(update.Message, bt)

			err := SendMsgToChannel(bot, update.Message.Chat.ID, reply, update.Message.MessageID)
			if err != nil {
				logrus.Panic(err)
			}
		case up := <-bt.Up2tel:
			logrus.WithField("key", up.Key).Debug("recv")

			err := SendMsgToUser(bot, up.Key.User, up.RSS.Content)
			if err != nil {
				logrus.Panic(err)
			}

			logrus.WithField("key", up.Key).Debug("saving")
			pubVal := model.JobValue{Published: *up.RSS.PublishedParsed, Processed: time.Now()}
			err = pudge.Set(model.DBPathJobs, up.Key.Key(), pubVal)
			if err != nil {
				logrus.Panic(err)
			}
		case msg := <-bt.Admin:
			err := SendMsgToUser(bot, config.GetAdmin(), "<b>Admin Message</b>\n"+msg)
			if err != nil {
				logrus.Panic(err)
			}
		case <-bt.Ctx.Done():
			logrus.Debug("telegram: done")
			err := SendMsgToUser(bot, config.GetAdmin(), "bot is going down")
			if err != nil {
				logrus.Panic(err)
			}
			return
		}
	}
}
