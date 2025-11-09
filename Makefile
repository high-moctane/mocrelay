SHELL := /bin/bash

TARGET_DIR := cmd/mocrelay
TARGET := $(TARGET_DIR)/mocrelay


.PHONY: all
all:
	$(MAKE) build


.PHONY: build
build: $(TARGET)


$(TARGET):
	cd $(TARGET_DIR) && go build


.PHONY: run
run: $(TARGET)
	$(TARGET)


.PHONY: check
check:
	go vet ./...
	test -z "$$(go tool goimports -l -local github.com/high-moctane/mocrelay .)"
	test -z "$$(go tool gofumpt -l .)"


.PHONY: test
test:
	go test ./...


.PHONY: bench
bench:
	go test -bench . -benchmem


.PHONY: clean
clean:
	$(RM) $(TARGET)


.PHONY: fmt
fmt:
	command -v modernize >/dev/null 2>&1 && modernize -fix -test ./... || true
	go tool goimports -w -l -local github.com/high-moctane/mocrelay .
	go tool gofumpt -w -l .


.PHONY: setup
setup:
	go tool lefthook install
