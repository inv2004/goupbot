package main

import (
	"fmt"
	"log"
	"sort"
	"time"

	upbot "github.com/inv2004/goupbot/internal/upbot"
	"github.com/recoilme/pudge"
)

type JobDesc struct {
	Processed time.Time
	Published time.Time
	GUID      string
}

func dumpUsers() {
	fmt.Printf("%s:\n", upbot.DBPathUsers)

	keys, err := pudge.Keys(upbot.DBPathUsers, nil, 0, 0, true)
	if err != nil {
		log.Panic(err)
	}

	for _, k := range keys {
		v := upbot.UserInfo{}

		err := pudge.Get(upbot.DBPathUsers, string(k), &v)
		if err != nil {
			log.Panic(err)
		}

		fmt.Printf("  \"%s\" %v\n", k, v)
	}
}

func dumpJobs() {
	fmt.Printf("%s:\n", upbot.DBPathJobs)

	keys, err := pudge.Keys(upbot.DBPathJobs, nil, 0, 0, true)
	if err != nil {
		log.Panic(err)
	}

	data := make([]JobDesc, len(keys))

	for _, k := range keys {
		v := upbot.JobValue{}
		err := pudge.Get(upbot.DBPathJobs, k, &v)
		if err != nil {
			log.Panic(err)
		}

		data = append(data, JobDesc{v.Processed, v.Published, string(k)})
	}

	sort.Slice(data, func(i, j int) bool {
		return data[i].Processed.Before(data[j].Processed)
	})

	for _, v := range data {
		fmt.Printf("  %s:%s %s\n", v.Processed, v.Published, v.GUID)
	}

}

func main() {
	dumpUsers()
	dumpJobs()
}
