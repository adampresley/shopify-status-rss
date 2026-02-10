.DEFAULT_GOAL := help
.PHONY: help

VERSION=$(shell cat ./VERSION)

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

build: ## Build the application
	CGO_ENABLED=0 go build -ldflags="-X 'main.Version=${VERSION}'" -mod=mod -o shopify-status-rss .

tag: ## Create a new release tag and push to Docker Hub
	docker build -t adampresley/shopify-status-rss:${VERSION} .
	docker build -t adampresley/shopify-status-rss:latest .
	docker push adampresley/shopify-status-rss:${VERSION}
	docker push adampresley/shopify-status-rss:latest
