
COVERDIR = .coverage
TOOLDIR = tools
BINDIR = bin
RELEASEDIR = release
CMD_DIR = cmd
DIRS = $(BINDIR) $(RELEASEDIR)

# Product name is the overall product name.
PRODUCT_NAME ?= $(shell basename $(shell pwd))

# GO_SRC is used to track source code changes for builds
GO_SRC := $(shell find . -name '*.go' ! -path '*/vendor/*' ! -path 'tools/*' ! -path 'bin/*' ! -path 'release/*' )
# GO_DIRS is used to pass package lists to gometalinter
GO_DIRS := $(shell find . -path './vendor/*' -o -path './tools/*' -o -name '*.go' -printf "%h\n" | uniq | tr -s '\n' ' ')
# GO_PKGS is used to run tests.
GO_PKGS := $(shell go list ./... | grep -v '/vendor/')
# GO_CMDS is used to build command binaries (by convention assume to be anything under cmd/)
GO_CMDS := $(shell find $(CMD_DIR) -mindepth 1 -type d -printf "%f ")

VERSION ?= $(shell git describe --dirty 2>/dev/null)
VERSION_SHORT ?= $(shell git describe --abbrev=0 2>/dev/null)

ifeq ($(VERSION),)
VERSION := v0.0.0
endif

ifeq ($(VERSION_SHORT),)
VERSION_SHORT := v0.0.0
endif

# List all go platforms supported on current system and filter down to common ones.
platforms := $(subst /,-,$(shell go tool dist list | \
	grep -e linux -e windows -e darwin | \
	grep -e 386 -e amd64))

# Build the list of binaries we can generate by combining cmds with platforms
PLATFORM_BINS := $(foreach cmd,$(GO_CMDS),$(patsubst %,$(BINDIR)/$(PRODUCT_NAME)_$(VERSION_SHORT)_%/$(cmd),$(platforms)))
PLATFORM_DIRS := $(patsubst %,$(BINDIR)/$(PRODUCT_NAME)_$(VERSION_SHORT)_%,$(platforms))
PLATFORM_TARS := $(patsubst %,$(RELEASEDIR)/$(PRODUCT_NAME)_$(VERSION_SHORT)_%.tar.gz,$(platforms))

# These are evaluated on use, and so will have the correct values in the build
# rule. Note that what happens is a PLATFORM_BIN name is post-processed by its dirname
# down to a GOOS and GOARCH params.
PLATFORMS_TEMP = $(subst -, ,$(subst /, ,$(patsubst $(BINDIR)/$(PRODUCT_NAME)_$(VERSION_SHORT)_%,%,$@)))
GOOS = $(word 1, $(PLATFORMS_TEMP))
GOARCH = $(word 2, $(PLATFORMS_TEMP))

# Helper to allow building just the current platform's binaries.
CURRENT_PLATFORM_BINS := $(foreach cmd,$(GO_CMDS),$(BINDIR)/$(PRODUCT_NAME)_$(VERSION_SHORT)_$(shell go env GOOS)-$(shell go env GOARCH)/$(cmd))

# CONCURRENT_LINTERS is useful on CI services which do not have the number of
# cores available that they advertise.
CONCURRENT_LINTERS ?=
ifeq ($(CONCURRENT_LINTERS),)
CONCURRENT_LINTERS = $(shell gometalinter --help | grep -o 'concurrency=\w*' | cut -d= -f2 | cut -d' ' -f1)
endif

# LINTER_DEADLINE should be increased on CI services.
LINTER_DEADLINE ?= 30s

# Ensure output dirs always exist.
$(shell mkdir -p $(DIRS))

# Ensure built tools are findable using the Makefile.
export PATH := $(TOOLDIR)/bin:$(PATH)
SHELL := env PATH=$(PATH) /bin/bash

all: style lint test binary

binary: $(GO_CMDS)

$(GO_CMDS): $(CURRENT_PLATFORM_BINS)
	ln -sf $< $@

$(PLATFORM_BINS): $(GO_SRC)
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -a \
		-ldflags "-extldflags '-static' -X main.Version=$(VERSION)" \
		-o $@ ./$(CMD_DIR)/$(shell basename $@)

$(PLATFORM_DIRS): $(PLATFORM_BINS)

$(PLATFORM_TARS): $(RELEASEDIR)/%.tar.gz : $(BINDIR)/%
	tar -czf $@ -C $(BINDIR) $$(basename $<)
	
release-bin: $(PLATFORM_BINS)

release: $(PLATFORM_TARS)

style: tools
	gometalinter --disable-all --enable=gofmt --enable=goimports --vendor $(GO_DIRS)

lint: tools
	@echo Using $(CONCURRENT_LINTERS) processes
	gometalinter -j $(CONCURRENT_LINTERS) \
		--deadline=$(LINTER_DEADLINE) \
		--enable-all \
		--line-length=120 \
		--disable=testify --disable=test --disable=goimports --disable=gofmt \
		--disable=gotype $(GO_DIRS)

fmt: tools
	gofmt -s -w $(GO_SRC)
	goimports -w $(GO_SRC)

test: tools
	@mkdir -p $(COVERDIR)
	@rm -f $(COVERDIR)/*
	for pkg in $(GO_PKGS) ; do \
		go test -v -covermode count \
			-coverprofile=$(COVERDIR)/$$(echo $$pkg | tr '/' '-').out $$pkg || \
			exit 1 ; \
	done
	gocovmerge $(shell find $(COVERDIR) -name '*.out') > cover.out

clean:
	[ ! -z $(BINDIR) ] && [ -e $(BINDIR) ] && find $(BINDIR) -print -delete || /bin/true
	[ ! -z $(COVERDIR) ] && [ -e $(COVERDIR) ] && find $(COVERDIR) -print -delete || /bin/true
	[ ! -z $(RELEASEDIR) ] && [ -e $(RELEASEDIR) ] && find $(RELEASEDIR) -print -delete || /bin/true
	rm -f $(GO_CMDS) || /bin/true
tools:
	$(MAKE) -C $(TOOLDIR)

autogen:
	@echo "Installing git hooks in local repository..."
	ln -sf $(TOOLDIR)/pre-commit .git/hooks/pre-commit

.PHONY: tools autogen style fmt test all release binary clean
