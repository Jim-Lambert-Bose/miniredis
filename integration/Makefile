.PHONY: all test

all: test

test:
	go test -tags int

commands.txt: ../*.go
	grep Register ../*.go|perl -ne '/"(.*)"/ && print "$$1\n"' | sort > commands.txt

