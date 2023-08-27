SHELL := /bin/bash

EXEDIR := cmd/mocrelay
TARGET := $(EXEDIR)/mocrelay

.PHONY: all
all:
	$(MAKE) build


.PHONY: build
build: $(TARGET)


$(TARGET):
	cd $(EXEDIR) && go build


.PHONY: run
run: $(TARGET)
	$(TARGET)


.PHONY: check
check:
	find . -name \*.go -exec goimports -l -v -local "github.com/high-moctane/mocrelay" {} \;
	go vet ./...
	# staticcheck ./...


.PHONY: test
test:
	go test ./...


.PHONY: clean
clean:
	$(RM) $(TARGET)


.PHONY: fmt
fmt:
	find . -name \*.go -exec goimports -w -v -local "github.com/high-moctane/mocrelay" {} \;


.PHONY: githook
githook:
	lefthook install


.PHONY: tool
tool:
	go install golang.org/x/tools/cmd/goimports@latest
