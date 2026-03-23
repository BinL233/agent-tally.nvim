BINARY   := sidecar/build/agent-tallyd
GOFLAGS  := -trimpath -ldflags="-s -w"

.PHONY: build clean install

build:
	cd sidecar && go build $(GOFLAGS) -o build/agent-tallyd ./cmd/agent-tallyd

clean:
	rm -rf sidecar/build

install: build
	cp $(BINARY) /usr/local/bin/agent-tallyd
