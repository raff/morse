LDFLAGS := -ldflags="-linkmode=external"

.PHONY: build run clean

build:
	go build $(LDFLAGS) -o mt .

run: build
	./mt $(ARGS)

clean:
	rm -f mt
