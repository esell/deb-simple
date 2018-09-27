BINARY := deb-simple
VERSION ?=master

all: test build

.PHONY : test
test:
	go test -v
clean:
	go clean
	rm -r release
build:
	go build -o $(BINARY)

.PHONY: release
release: build-win build-linux build-osx

build-win:
	GOOS=windows go build -o release/$(BINARY)-$(VERSION)-win.exe
build-linux:
	GOOS=linux go build -o release/${BINARY}-$(VERSION)-linux-amd64
build-osx:
	GOOS=darwin go build -o release/$(BINARY)-$(VERSION)-osx

