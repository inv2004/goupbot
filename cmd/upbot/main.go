package main

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/inv2004/goupbot/internal/upbot/bot"
	"github.com/inv2004/goupbot/internal/upbot/model"
	"github.com/inv2004/goupbot/internal/upbot/telegram"
	"github.com/inv2004/goupbot/internal/upbot/upwork"
	"github.com/recoilme/pudge"
	"github.com/sirupsen/logrus"
	// "github.com/upwork/golang-upwork/api"
	// "github.com/upwork/golang-upwork/api/routers/jobs"
	// "github.com/upwork/golang-upwork/api/routers/jobs/search"
)

func main() {
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	if len(os.Args) == 2 && os.Args[1] == "migrate" {
		// telegram.MigrateUserId()
		telegram.MigrateOneUser()
		return
	}

	ctx, cancel := context.WithCancel(context.Background())

	bt := &bot.BotStruct{
		Wg:     &sync.WaitGroup{},
		Ctx:    ctx,
		Up2tel: make(chan model.JobInfo),
		Admin:  make(chan string),
	}

	defer func() {
		err := pudge.CloseAll()
		if err != nil {
			logrus.Panic(err)
		}
		logrus.Info("db closed")
	}()

	bt.Wg.Add(2)
	go telegram.Start(bt)
	go upwork.Start(bt)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	<-quit
	logrus.Warn("signal to shutdown ...")
	cancel()
	bt.Wg.Wait()
}
