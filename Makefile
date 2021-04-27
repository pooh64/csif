.PHONY: all plugin filter clean

all: plugin filter

clean:
	rm -rf ./bin

filter:
	mkdir -p ./bin
	go build -mod vendor -o ./bin/csif-filter ./cmd/csif-filter

plugin:
	mkdir -p ./bin
	go build -mod vendor -o ./bin/csif-plugin ./cmd/csif-plugin