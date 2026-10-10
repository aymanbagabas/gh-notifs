.PHONY: build test check example

build:
	go build -o bin/gh-notifs .

test:
	go test -race ./...

check:
	go vet ./...

example:
	go run ./examples/query
	go run ./examples/tsv
	go run ./examples/theme
	go run ./examples/theme --light
	go run ./examples/config
	go run ./examples/preview
	go run ./examples/discussion
	go run ./examples/discussion --threads
