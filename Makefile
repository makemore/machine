BINARY  := mach
INSTALL := /usr/local/bin

.PHONY: build install uninstall test clean release

build:
	go build -o $(BINARY) .

install: build
	@mkdir -p $(INSTALL)
	@cp $(BINARY) $(INSTALL)/$(BINARY)
	@echo "✅ $(BINARY) installed to $(INSTALL)/$(BINARY)"

uninstall:
	@rm -f $(INSTALL)/$(BINARY)
	@echo "🗑  $(BINARY) removed from $(INSTALL)"

test:
	go test ./...

clean:
	rm -f $(BINARY)
	rm -rf dist/

release: clean
	GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o dist/$(BINARY)-darwin-arm64 .
	GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o dist/$(BINARY)-darwin-amd64 .
	GOOS=linux  GOARCH=arm64 go build -ldflags="-s -w" -o dist/$(BINARY)-linux-arm64  .
	GOOS=linux  GOARCH=amd64 go build -ldflags="-s -w" -o dist/$(BINARY)-linux-amd64  .
	@ls -lh dist/

