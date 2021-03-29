all:
	mkdir -p ./bin
	go build -mod vendor -o ./bin/cmd ./cmd

clean:
	rm -rf ./bin