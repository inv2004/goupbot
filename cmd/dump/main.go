package main

import (
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/inv2004/goupbot/internal/upbot/model"
	"github.com/recoilme/pudge"
)

type JobDesc struct {
	Processed time.Time
	Published time.Time
	GUID      string
}

func dumpUsers() {
	fmt.Printf("%s:\n", model.DBPathUsers)

	keys, err := pudge.Keys(model.DBPathUsers, nil, 0, 0, true)
	if err != nil {
		log.Panic(err)
	}

	for _, k := range keys {
		v := model.UserInfo{}

		err := pudge.Get(model.DBPathUsers, string(k), &v)
		if err != nil {
			log.Panic(err)
		}

		fmt.Printf("  \"%s\" %v\n", k, v)
	}
}

func dumpJobs() {
	fmt.Printf("%s:\n", model.DBPathJobs)

	keys, err := pudge.Keys(model.DBPathJobs, nil, 0, 0, true)
	if err != nil {
		log.Panic(err)
	}

	data := make([]JobDesc, len(keys))

	for _, k := range keys {
		v := model.JobValue{}
		err := pudge.Get(model.DBPathJobs, k, &v)
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
