package bot

import (
	"context"
	"sync"

	"github.com/inv2004/goupbot/internal/upbot/model"
)

type BotStruct struct {
	Wg     *sync.WaitGroup
	Ctx    context.Context
	Up2tel chan model.JobInfo
	Admin  chan string
}
