SHELL := /bin/bash

EXEDIR := cmd/mocrelay
TARGET := $(EXEDIR)/mocrelay

.PHONY: all
all:
	$(MAKE) build


.PHONY: build
build: $(TARGET)


$(TARGET): *.go cmd/*/*.go
	cd $(EXEDIR) && go build


.PHONY: run
run: $(TARGET)
	$(TARGET)


.PHONY: check
check:
	test -z "$$(find . -name \*.go | xargs -P 8 -I {} golines -l {} 2>&1 | tee /dev/stderr)"
	go vet ./...
	staticcheck ./...


.PHONY: test
test:
	go test ./...


.PHONY: clean
clean:
	$(RM) $(TARGET)


.PHONY: fmt
fmt:
	find . -name \*.go | xargs -P 8 -I {} golines -w {}


.PHONY: githook
githook:
	lefthook install


.PHONY: tool
tool:
	go install github.com/segmentio/golines@latest
	go install golang.org/x/tools/cmd/goimports@latest
	go install honnef.co/go/tools/cmd/staticcheck@latest
