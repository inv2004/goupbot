package telegram

import (
	"errors"
	"fmt"
	"os"
	"strconv"
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

const AdminMessage = "<b>Admin Message</b>\n"

const imgUrl = "./rss.png"

func sendWhere(bot *tgbotapi.BotAPI, channel int64, replyTo int) (err error) {
	pic := tgbotapi.NewPhotoUpload(channel, imgUrl)

	if replyTo > 0 {
		pic.ReplyToMessageID = replyTo
	}

	_, err = bot.Send(pic)
	if err != nil {
		return err
	}

	msg := tgbotapi.NewMessage(channel, "paste rss url to add here:")
	_, err = bot.Send(msg)
	if err != nil {
		return err
	}

	return nil
}

func escapeHtml(s string) (result string) {
	result = s
	result = strings.ReplaceAll(result, "<", "&lt;")
	result = strings.ReplaceAll(result, ">", "&gt;")
	return
}

func SendMsgToChannel(bot *tgbotapi.BotAPI, channel int64, text string, replyTo int) (err error) {
	if text == "/where" {
		return sendWhere(bot, channel, replyTo)
	}

	text = strings.ReplaceAll(text, "<br />", "\n")
	text = strings.ReplaceAll(text, "<br/>", "\n")
	text = strings.ReplaceAll(text, "<br>", "\n")
	text = strings.ReplaceAll(text, "&nbsp;", " ")
	text = strings.ReplaceAll(text, "&quot;", `"`)
	text = strings.ReplaceAll(text, "&amp;", "&")

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
	userId := fmt.Sprintf("%d", msg.From.ID)
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
		err := pudge.Get(model.DBPathUsers, userId, &userInfo)
		if err != nil {
			if errors.Is(err, pudge.ErrKeyNotFound) {
				userInfo.UserName = msg.From.UserName
				userInfo.ChannelID = msg.Chat.ID
				userInfo.Feeds = []model.FeedInfo{}
			}
		}

		if userInfo.Active {
			reply = "Your user is active already"
			return
		}

		userInfo.Active = true
		userInfo.Pull = config.GetDelay()
		err = pudge.Set(model.DBPathUsers, userId, userInfo)
		if err != nil {
			logrus.Panic(err)
		}
		feedInfo := ""
		if upwork.HasActiveFeeds(&userInfo) {
			feedInfo = "You have some channels already, check with /list\n\n"
			bt.Wg.Add(1)
			go upwork.FetchUser(userInfo.UserName, bt)
		}

		reply = "Thank you for subscribing the bot.\n\n" + feedInfo + "Please add feed channels by /add command or /help for help"
	case "/stop":
		userInfo := model.UserInfo{}
		err := pudge.Get(model.DBPathUsers, userId, &userInfo)
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
		logrus.WithField("user", userId).WithField("userInfo", userInfo).Debug("Store")
		err = pudge.Set(model.DBPathUsers, userId, &userInfo)
		if err != nil {
			logrus.Panic(err)
		}
		bt.Stop2user <- msg.From.UserName
		reply = "Your user and feeds are suspended, Type /start to resume"
	case "/ping":
		reply = "pong"
	case "/add":
		userInfo := model.UserInfo{}
		err := pudge.Get(model.DBPathUsers, userId, &userInfo)
		if err != nil {
			if errors.Is(err, pudge.ErrKeyNotFound) {
				reply = "Type /start first"
				return
			} else {
				logrus.Panic(err)
			}
		}
		if !userInfo.Active {
			reply = "Type /start to resume"
			return
		}
		userInfo.WaitingFeedUrl = model.WaitingAdd
		err = pudge.Set(model.DBPathUsers, userId, userInfo)
		if err != nil {
			logrus.Panic(err)
		}
		reply = `To help find rss on upwork: /where.<br/>
		or paste rss URL to add here:`
	case "/where":
		reply = "/where"
		return
	case "/del":
		userInfo := model.UserInfo{}
		err := pudge.Get(model.DBPathUsers, userId, &userInfo)
		if err != nil {
			if errors.Is(err, pudge.ErrKeyNotFound) {
				reply = "Type /start first"
				return
			} else {
				logrus.Panic(err)
			}
		}
		userInfo.WaitingFeedUrl = model.WaitingDel
		err = pudge.Set(model.DBPathUsers, userId, userInfo)
		if err != nil {
			logrus.Panic(err)
		}
		reply = "Paste rss number to delete here:"
	case "/list":
		userInfo := model.UserInfo{}
		err := pudge.Get(model.DBPathUsers, userId, &userInfo)
		if err != nil {
			if errors.Is(err, pudge.ErrKeyNotFound) {
				reply = "Type /start first"
				return
			} else {
				logrus.Panic(err)
			}
		}

		for i, v := range userInfo.Feeds {
			if !v.IsActive {
				continue
			}
			reply += fmt.Sprintf(`%d) <a href="%s">%s</a><br/>`, i+1, v.Url, v.Title)
			i += 1
		}
		if len(userInfo.Feeds) == 0 {
			reply = "Empty"
		}
	case "/pull 1m":
		userInfo := model.UserInfo{}
		err := pudge.Get(model.DBPathUsers, userId, &userInfo)
		if err != nil {
			logrus.Panic(err)
		}
		userInfo.Pull = 60 * time.Second
		err = pudge.Set(model.DBPathUsers, userId, userInfo)
		if err != nil {
			logrus.Panic(err)
		}
		reply = "ok"
	default:
		userInfo := model.UserInfo{}
		err := pudge.Get(model.DBPathUsers, userId, &userInfo)
		if err != nil {
			logrus.Panic(err)
		}
		switch userInfo.WaitingFeedUrl {
		case model.WaitingAdd:
			title, err := upwork.AddChannel(userId, text, bt)
			if err != nil {
				reply = escapeHtml(err.Error())
				return
			}

			reply = fmt.Sprintf("<b>%s</b> added succesfully. Default pull interval is 5 minutes. You will receive new jobs from the moment", title)
		case model.WaitingDel:
			idx, err := strconv.Atoi(text)
			if err != nil {
				reply = "number expected"
				return
			}
			err = upwork.DelChannel(userId, idx-1, bt)
			if err != nil {
				reply = escapeHtml(err.Error())
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

	defer func() {
		SendMsgToUser(bot, config.GetAdmin(), AdminMessage+"bot is going down")
	}()

	logrus.WithField("bot", bot.Self.UserName).Info("Authorized on account")

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates, err := bot.GetUpdatesChan(u)
	if err != nil {
		logrus.Panic(err)
	}

	err = SendMsgToUser(bot, config.GetAdmin(), AdminMessage+"bot is up")
	if err != nil {
		logrus.Warn(err)
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
			err := SendMsgToUser(bot, config.GetAdmin(), AdminMessage+msg)
			if err != nil {
				logrus.Panic(err)
			}
		case <-bt.Ctx.Done():
			logrus.Debug("telegram: done")
			err := SendMsgToUser(bot, config.GetAdmin(), AdminMessage+"bot is going down")
			if err != nil {
				logrus.Panic(err)
			}
			return
		}
	}
}

func MigrateUserId() {
	keys, err := pudge.Keys(model.DBPathUsers, nil, 0, 0, true)
	if err != nil {
		logrus.Panic(err)
	}

	db, err := pudge.Open(model.DBPathUsers, &pudge.Config{})
	if err != nil {
		panic(err)
	}
	defer db.Close()

	m := map[string]string{}

	for _, user := range keys {
		u := string(user)
		logrus.Println(u)
		userInfo := model.UserInfo{}
		err := db.Get(user, &userInfo)
		if err != nil {
			panic(err)
		}
		userInfo.UserName = u
		// userInfo.Feeds = userInfo.Feeds[0:0]
		logrus.Infof("%+v", userInfo)
		db.Delete(u)
		logrus.Infof("%+v", userInfo)
		userInfo.UserName = u
		db.Set(fmt.Sprintf("%d", userInfo.ChannelID), userInfo)
		m[u] = fmt.Sprintf("%d", userInfo.ChannelID)
	}

	keys, err = pudge.Keys(model.DBPathJobs, nil, 0, 0, true)
	if err != nil {
		logrus.Panic(err)
	}

	db2, err := pudge.Open(model.DBPathJobs, &pudge.Config{})
	if err != nil {
		panic(err)
	}
	defer db2.Close()
	for _, jobB := range keys {
		job := string(jobB)
		logrus.Println(job)
		jobValue := model.JobValue{}
		fmt.Println("DDD0: ", job)
		err := db2.Get(job, &jobValue)
		if err != nil {
			panic(err)
		}
		logrus.Infof("%+v", jobValue)
		db2.Delete(job)
		kk := strings.Split(string(job), ";")
		newK := model.JobInfoKey{User: m[kk[0]], GUID: kk[1]}.Key()
		fmt.Println("DDD1: ", newK)
		db2.Set(newK, jobValue)
	}

}
