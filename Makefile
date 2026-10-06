IMG ?= staticsite-operator:dev
KIND_CLUSTER ?= staticsite
CONTROLLER_GEN ?= go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.16.5

.PHONY: all
all: test build

##@ Development

.PHONY: generate
generate: ## Generate DeepCopy methods for the API types.
	$(CONTROLLER_GEN) object paths=./api/v1alpha1

.PHONY: manifests
manifests: ## Generate the CRD and RBAC role from kubebuilder markers.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd \
		paths=./api/v1alpha1 paths=./internal/controller \
		output:crd:artifacts:config=config/crd/bases \
		output:rbac:artifacts:config=config/rbac

.PHONY: test
test: generate manifests ## Run unit tests.
	go vet ./...
	go test ./... -coverprofile cover.out

.PHONY: build
build: generate ## Build the manager binary.
	go build -o bin/manager ./cmd

.PHONY: run
run: manifests generate ## Run the controller locally against the current kubeconfig context.
	kubectl apply -k config/crd
	go run ./cmd

##@ Cluster

.PHONY: kind-create
kind-create: ## Create a local kind cluster.
	kind create cluster --name $(KIND_CLUSTER)

.PHONY: docker-build
docker-build: ## Build the operator image.
	docker build -t $(IMG) .

.PHONY: kind-load
kind-load: docker-build ## Load the operator image into kind.
	kind load docker-image $(IMG) --name $(KIND_CLUSTER)

.PHONY: deploy
deploy: manifests ## Install the CRD, RBAC and controller into the cluster.
	kubectl apply -k config/default

.PHONY: undeploy
undeploy: ## Remove everything installed by deploy.
	kubectl delete -k config/default --ignore-not-found

.PHONY: e2e
e2e: ## Run the end-to-end script against the current cluster (expects deploy to have run).
	./hack/e2e.sh

.PHONY: help
help: ## Show this help.
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*?##/ { printf "  %-14s %s\n", $$1, $$2 } /^##@/ { printf "\n%s\n", substr($$0, 5) }' $(MAKEFILE_LIST)
