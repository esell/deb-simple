BINARY := deb-simple
VERSION ?=master

all: test build

.PHONY : test
test:
	go test -v
clean:
	go clean
	rm -rf release
build:
	dep ensure
	go build -o $(BINARY)

.PHONY: release
release: build-win build-linux build-osx build-deb

build-win:
	GOOS=windows go build -o release/$(BINARY)-$(VERSION)-win.exe
build-linux:
	GOOS=linux go build -o release/${BINARY}-$(VERSION)-linux-amd64
build-osx:
	GOOS=darwin go build -o release/$(BINARY)-$(VERSION)-osx
build-deb:
	which -s dpkg-deb || { echo "dpkg-deb does not exist, exiting..."; exit 1; }
	mkdir release/$(BINARY)-$(VERSION)
	mkdir -p release/$(BINARY)-$(VERSION)/usr/local/bin
	mkdir -p release/$(BINARY)-$(VERSION)/etc/deb-simple
	cp -r DEBIAN release/$(BINARY)-$(VERSION)/
	cp sample_conf.json release/$(BINARY)-$(VERSION)/etc/deb-simple/conf.json
	GOOS=linux go build -o release/$(BINARY)-$(VERSION)/usr/local/bin/${BINARY}
	dpkg-deb --build release/$(BINARY)-$(VERSION)
