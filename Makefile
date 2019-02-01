
install: pubsublistener ## install the project binaries
	go install chainspace.io/blockmania/cmd/blockmania

pubsublistener:
	go install chainspace.io/blockmania/cmd/pubsublistener

swaggerdocs: ## generate the swaggerdocs for the rest server
	rm -rf rest/api/docs && swag init -g rest/api/api.go && mv docs rest/api/docs

.PHONY: help

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
