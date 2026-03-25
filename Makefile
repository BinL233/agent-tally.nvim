BINARY    := sidecar/build/agent-tallyd
GOFLAGS   := -trimpath -ldflags="-s -w"
LOCAL_BIN := $(HOME)/.local/bin

.PHONY: build clean install

build:
	cd sidecar && go build $(GOFLAGS) -o build/agent-tallyd ./cmd/agent-tallyd

clean:
	rm -rf sidecar/build

install: build
	@mkdir -p $(LOCAL_BIN)
	@RC_FILE=""; \
	if [ -f "$(HOME)/.zshrc" ]; then RC_FILE="$(HOME)/.zshrc"; \
	elif [ -f "$(HOME)/.bashrc" ]; then RC_FILE="$(HOME)/.bashrc"; fi; \
	if [ -n "$$RC_FILE" ] && ! grep -q '\.local/bin' "$$RC_FILE" 2>/dev/null; then \
		echo 'export PATH="$$HOME/.local/bin:$$PATH"' >> "$$RC_FILE"; \
		echo "Added ~/.local/bin to PATH in $$RC_FILE (restart your shell or source it to apply)"; \
	fi
	@cp $(BINARY) $(LOCAL_BIN)/agent-tallyd 2>/dev/null && \
		echo "Installed agent-tallyd to $(LOCAL_BIN)" || \
		(echo "~/.local/bin install failed, falling back to /usr/local/bin (may require sudo)..." && \
		 cp $(BINARY) /usr/local/bin/agent-tallyd && \
		 echo "Installed agent-tallyd to /usr/local/bin")
