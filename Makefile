all:
	mkdir -p ./bin
	go build -mod vendor -o ./bin/csif-plugin ./cmd

clean:
	rm -rf ./bin