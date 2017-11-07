
NAME = cf-conduit
GOBUILD = go build
ALL_ARCH = amd64 386
ALL_GOOS = windows linux darwin

.PHONY: install
install: bindata.go vendor
	mkdir -p bin
	$(GOBUILD) -o bin/$(NAME)
	cf install-plugin -f bin/$(NAME)

.PHONY: dist
dist: bindata.go vendor
	mkdir -p bin
	for arch in $(ALL_ARCH); do \
		for platform in $(ALL_GOOS); do \
			GOOS=$$platform ARCH=$$arch $(GOBUILD) -o bin/$(NAME).$$arch.$$platform; \
		done; \
	done

bindata.go:
	go-bindata -o $@ -nocompress data/

vendor:
	dep ensure

.PHONY: clean
clean:
	rm -rf vendor
	rm -rf bin
	rm bindata.go



