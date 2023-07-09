LDFLAGS := "-s -w"

.PHONY: build
build:
	go build -ldflags=$(LDFLAGS) -o mocrelay


.PHONY: run
run:
	./mocrelay


.PHONY: all
all:
	$(MAKE) build
	$(MAKE) run
