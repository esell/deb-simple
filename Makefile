BINARY := deb-simple
VERSION := $(shell git describe --abbrev=0 --tags)

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
	dpkg-deb --version >/dev/null || { echo "dpkg-deb does not exist, exiting..."; exit 1; }
	sed -i "s/<VERSION>/$(VERSION)/" debroot/DEBIAN/control
	mkdir -p debroot/usr/bin
	mkdir -p debroot/srv/deb-simple/repo
	dep ensure
	GOOS=linux go build -o debroot/usr/bin/$(BINARY)
	dpkg-deb --build debroot $(BINARY)_$(VERSION).deb
