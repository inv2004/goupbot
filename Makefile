all: build build_dump

build:
	CGO_ENABLED=0 go build -o upbot cmd/upbot/main.go

build_dump:
	CGO_ENABLED=0 go build  -o dump cmd/dump/main.go

run:
	go run cmd/upbot/main.go

dump:
	go run cmd/dump/main.go

migrate:
	go run cmd/upbot/main.go migrate
