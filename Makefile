.PHONY: build
build:
	go build -o mocrelay


.PHONY: run
run:
	./mocrelay


.PHONY: all
all: build run
