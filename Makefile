# Aalsi Makefile

.PHONY: run test eval demo-data build clean purge

run:
	go run ./cmd/aalsi

run-lan:
	go run ./cmd/aalsi --lan

build:
	go build -o aalsi.exe ./cmd/aalsi

test:
	go test -v ./...

eval:
	go run ./cmd/aalsi eval

demo-data:
	go run ./cmd/aalsi demo-data

purge:
	go run ./cmd/aalsi purge

clean:
	rm -f aalsi aalsi.exe
