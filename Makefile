all: gograz-meetup

gograz-meetup: $(shell find . -name '*.go')
	go build

clean:
	rm -f gograz-meetup

.PHONY: clean all