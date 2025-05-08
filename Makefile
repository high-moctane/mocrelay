SHELL := /bin/bash

EXEDIR := cmd/mocrelay
TARGET := $(EXEDIR)/mocrelay

.PHONY: all
all:
	$(MAKE) build


.PHONY: build
build: $(TARGET)


$(TARGET): $(shell find . -name \*.go)
	cd $(EXEDIR) && go build


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
	find . -name \*.go | xargs -P 8 -I {} go tool golines -w {}


.PHONY: setup
setup:
	go install tool
	go tool lefthook install
