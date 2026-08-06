.PHONY: frontend build test docker-build fmt vet check lint

APP_DIR := github_runner
GO_PKGS := ./cmd/... ./internal/... ./api/...

frontend:
	cd $(APP_DIR)/frontend && (rm -rf node_modules 2>/dev/null || true) && npm ci && npm run build
	rm -rf $(APP_DIR)/cmd/github-runner-addon/frontend-dist
	cp -r $(APP_DIR)/frontend/dist $(APP_DIR)/cmd/github-runner-addon/frontend-dist

build:
	./scripts/build.sh

test:
	cd $(APP_DIR) && go test $(GO_PKGS)

frontend-test:
	cd $(APP_DIR)/frontend && npm test

fmt:
	gofmt -w $$(find $(APP_DIR) -name '*.go' -not -path '*/frontend/*')

vet:
	cd $(APP_DIR) && go vet $(GO_PKGS)

lint:
	cd $(APP_DIR)/frontend && npm ci && npm run lint

check: vet test

docker-build:
	docker build -t ghcr.io/dchote/github-runner-addon:local ./$(APP_DIR)
