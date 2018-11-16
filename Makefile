
NAME = cf-conduit
GOBUILD = go build
ALL_GOARCH = amd64 386
ALL_GOOS = windows linux darwin

.PHONY: build
build:
	mkdir -p bin
	$(GOBUILD) -o bin/$(NAME)

.PHONY: install
install: build
	cf install-plugin -f bin/$(NAME)

.PHONY: dist
dist:
	$(eval export NAME)
	$(eval export GOBUILD)
	$(eval export ALL_GOARCH)
	$(eval export ALL_GOOS)
	./dist.sh

.PHONY: clean
clean:
	rm -rf bin

.PHONY: test
test:
	go vet
	go test $(go list ./... | grep -v /vendor/)
