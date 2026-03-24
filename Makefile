BINARY  := mach
INSTALL := /usr/local/bin
GCS_BUCKET := manyhands-mach-releases

PLATFORMS := darwin-arm64 darwin-amd64 linux-arm64 linux-amd64

.PHONY: build install uninstall test clean release upload release-upload

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

upload:
	@echo "📤 Uploading mach binaries to gs://$(GCS_BUCKET)/latest/..."
	@for f in $(PLATFORMS); do \
		if [ -f dist/$(BINARY)-$$f ]; then \
			gsutil -h "Cache-Control:public, max-age=300" cp dist/$(BINARY)-$$f gs://$(GCS_BUCKET)/latest/$(BINARY)-$$f; \
			echo "  ✅ $$f"; \
		else \
			echo "  ⚠️  dist/$(BINARY)-$$f not found, skipping"; \
		fi; \
	done
	@echo "📤 Done. Binaries at https://storage.googleapis.com/$(GCS_BUCKET)/latest/"

release-upload: release upload
	@echo "🚀 mach release complete!"

