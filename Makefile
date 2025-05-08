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
	test -z "$$(find . -name \*.go | xargs -P 8 -I {} go tool golines -l {} 2>&1 | tee /dev/stderr)"
	go vet ./...
	go tool staticcheck ./...


.PHONY: test
test:
	GODEBUG=randautoseed=0 go test ./...


.PHONY: bench
bench:
	GODEBUG=randautoseed=0 go test -bench . -benchmem


.PHONY: clean
clean:
	$(RM) $(TARGET)


.PHONY: fmt
fmt:
	go tool goimports -w -l -local github.com/high-moctane/mocrelay .
	go tool gofumpt -w -l .


.PHONY: setup
setup:
	go install tool
	go tool lefthook install
