all: build build_dump

build:
	go build  -o upbot cmd/upbot/main.go

build_dump:
	go build  -o dump cmd/dump/main.go

run:
	go run cmd/upbot/main.go

dump:
	go run cmd/dump/main.go

