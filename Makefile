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
	goimports -l -local mocrelay . $(EXEDIR)
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
	goimports -w -local mocrelay . $(EXEDIR)


.PHONY: githook
githook:
	lefthook install
