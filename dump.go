package main

import (
	"fmt"
	"log"

	upbot "./upbot"
	"github.com/recoilme/pudge"
)

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

		fmt.Printf("  %s %v\n", k, v)
	}
}

func dumpJobs() {
	fmt.Printf("%s:\n", upbot.DBPathJobs)

	keys, err := pudge.Keys(upbot.DBPathJobs, nil, 0, 0, true)
	if err != nil {
		log.Panic(err)
	}

	for _, k := range keys {
		v := upbot.UserInfo{}
		err := pudge.Get(upbot.DBPathJobs, k, &v)
		if err != nil {
			log.Panic(err)
		}

		fmt.Printf("  %s %v\n", k, v)
	}
}

func main() {
	dumpUsers()
	dumpJobs()
}
