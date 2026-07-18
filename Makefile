BINARY := drone-commander-driver
DIST := dist
LDFLAGS := -s -w
TAGS := migrated_fynedo
DARWIN_IMAGE ?= fyneio/fyne-cross-images:darwin
MACOS_SDK_PATH ?=

.PHONY: build test build-linux build-windows build-macos build-all clean

build:
	CGO_ENABLED=1 go build -trimpath -tags="$(TAGS)" -ldflags="$(LDFLAGS)" -o $(BINARY) .

test:
	go test ./...

build-linux:
	mkdir -p $(DIST)
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -trimpath -tags="$(TAGS)" -ldflags="$(LDFLAGS)" -o $(DIST)/$(BINARY)-linux-amd64 .

build-windows:
	mkdir -p $(DIST)
	CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc go build -trimpath -tags="$(TAGS)" -ldflags="$(LDFLAGS) -H=windowsgui" -o $(DIST)/$(BINARY)-windows-amd64.exe .

build-macos:
	@test -n "$(MACOS_SDK_PATH)" || (echo "Imposta MACOS_SDK_PATH alla directory MacOSX.sdk" && exit 1)
	mkdir -p $(DIST)
	docker run --rm --user "$$(id -u):$$(id -g)" -w /app -v "$(CURDIR):/app" -v "$(abspath $(MACOS_SDK_PATH)):/sdk:ro" -v "$(HOME)/.cache/fyne-cross:/go" -e HOME=/tmp -e CGO_ENABLED=1 -e GOCACHE=/go/go-build -e GOOS=darwin -e GOARCH=amd64 -e 'CC=zig cc -target x86_64-macos.10.12 -isysroot /sdk -iwithsysroot /usr/include -iframeworkwithsysroot /System/Library/Frameworks' -e 'CXX=zig c++ -target x86_64-macos.10.12 -isysroot /sdk -iwithsysroot /usr/include -iframeworkwithsysroot /System/Library/Frameworks' -e 'CGO_LDFLAGS=--sysroot /sdk -F/System/Library/Frameworks -L/usr/lib' $(DARWIN_IMAGE) go build -trimpath -tags=$(TAGS) -ldflags='$(LDFLAGS)' -o /app/$(DIST)/$(BINARY)-macos-amd64 .
	docker run --rm --user "$$(id -u):$$(id -g)" -w /app -v "$(CURDIR):/app" -v "$(abspath $(MACOS_SDK_PATH)):/sdk:ro" -v "$(HOME)/.cache/fyne-cross:/go" -e HOME=/tmp -e CGO_ENABLED=1 -e GOCACHE=/go/go-build -e GOOS=darwin -e GOARCH=arm64 -e 'CC=zig cc -target aarch64-macos.11 -isysroot /sdk -iwithsysroot /usr/include -iframeworkwithsysroot /System/Library/Frameworks' -e 'CXX=zig c++ -target aarch64-macos.11 -isysroot /sdk -iwithsysroot /usr/include -iframeworkwithsysroot /System/Library/Frameworks' -e 'CGO_LDFLAGS=--sysroot /sdk -F/System/Library/Frameworks -L/usr/lib' $(DARWIN_IMAGE) go build -trimpath -tags=$(TAGS) -ldflags='$(LDFLAGS)' -o /app/$(DIST)/$(BINARY)-macos-arm64 .

build-all: build-linux build-windows build-macos

clean:
	rm -rf $(DIST) $(BINARY) fyne-cross
