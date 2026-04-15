LDFLAGS := -ldflags="-linkmode=external"

# Package source files excluding main.go, test_timing.go, *_test.go, and the
# platform file that does NOT match the current OS.  go run with explicit file
# lists ignores build constraints, so we must filter manually.
UNAME := $(shell uname)
ifeq ($(UNAME),Darwin)
  EXCLUDE_PLATFORM := input_send_other.go
else
  EXCLUDE_PLATFORM := input_send_darwin.go
endif
PKG_SRCS := $(filter-out main.go test_timing.go $(EXCLUDE_PLATFORM) $(wildcard *_test.go),$(wildcard *.go))

.PHONY: build run clean timing

build:
	go build $(LDFLAGS) -o mt .

run: build
	./mt $(ARGS)

# Run the timing diagnostic: press [ / ] (or paddle Ctrl keys) and watch
# raw KeyEvents + IambicAdapter outputs with millisecond timestamps.
timing:
	go run $(LDFLAGS) $(PKG_SRCS) test_timing.go

clean:
	rm -f mt
