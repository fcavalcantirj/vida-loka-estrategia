.PHONY: build run

build:
	go build -o vida-loka-server cmd/server/main.go

run: build
	./vida-loka-server --config=./config/config.json

clean:
	rm -f vida-loka-server 