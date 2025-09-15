.DEFAULT_GOAL := help

##@ Help
.PHONY: help
help:  ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z0-9_-]+:.*?##/ { printf "  \033[36m%-25s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Commands
.PHONY: start
start:  ## Start docker stack
	mkdir -p .fileserver/data && rm -rf .fileserver/data/*
	docker-compose stop
	docker-compose up -d --remove-orphans

.PHONY: start-dev
start-dev:  ## Start docker stack in development mode (with debugpy enabled)
	mkdir -p .fileserver/data && rm -rf .fileserver/data/*
	docker-compose stop
	docker-compose -f docker-compose.dev.yml up -d --remove-orphans

.PHONY: start-clean
start-clean: clean start ## Stop, clean, rebuild, and start docker stack

.PHONY: start-clean-dev
start-clean-dev: clean-dev start-dev ## Stop, clean, rebuild, and start docker stack in development mode

.PHONY: start-clean-tail
start-clean-tail: clean start tail  ## Stop, clean, rebuild, start docker stack, and tail load test logs

.PHONY: start-tail
start-tail: start tail  ## Stop, restart docker stack, and tail load test logs

.PHONY: clean
clean: ## Stop & clean docker stack
	docker-compose stop  && docker-compose rm -f && docker-compose build --force-rm --parallel

.PHONY: clean-dev
clean-dev: ## Stop & clean docker stack including dev containers
	docker-compose -f docker-compose.dev.yml -f docker-compose.dev.yml stop  && docker-compose -f docker-compose.dev.yml -f docker-compose.dev.yml rm -f && docker-compose -f docker-compose.dev.yml -f docker-compose.dev.yml build --force-rm --parallel

.PHONY: stop
stop:  ## Stop docker stack
	docker-compose down

.PHONY: show-containers
show-containers: ## Show running container information
	docker ps

.PHONY: tail
tail:  ## Live tail logs of load test script
	docker logs -f load-tester

.PHONY: stats
stats: ## Show container CPU / Memory / IO Utilization
	docker stats

.PHONY: load-test
load-test: ## Manually execute python load test. REQUIRES PYTHON INSTALLATION
	./load_test/run.sh

.PHONY: run-middleware
run-middleware:
	cd go_middleware && go run main.go