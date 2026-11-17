# Cloud Optimization – compilation
#
#   make            -> binaires pour la machine locale  (bin/local/)
#   make linux      -> binaires pour les VM GCP x86_64  (bin/linux-amd64/)
#   make all        -> les deux
#   make vet        -> vérification statique du code Go
#   make clean      -> supprime bin/
#
# Pourquoi deux cibles ? Le Mac (et la VM OrbStack) est en ARM64, alors que les
# VM GCP e2-* sont en x86_64 : un binaire ARM ne s'exécute pas sur une VM e2.
# Go sait « cross-compiler » : on choisit l'OS et l'architecture cible avec
# GOOS / GOARCH, sans rien installer sur les VM.

WORKLOADS  := cpu_task memory_task matrix_task sort_task
LOCAL_DIR  := bin/local
LINUX_DIR  := bin/linux-amd64

# CGO_ENABLED=0 -> binaire statique, sans dépendance à la libc de la VM.
# -trimpath / -s -w -> binaires plus petits et reproductibles.
GOFLAGS_BUILD := -trimpath -ldflags="-s -w"

.PHONY: local linux tools all vet test clean

# Par défaut : workloads locaux + outils de la machine de contrôle.
local: tools
	@mkdir -p $(LOCAL_DIR)
	@for w in $(WORKLOADS); do \
		echo "build $(LOCAL_DIR)/$$w"; \
		CGO_ENABLED=0 go build $(GOFLAGS_BUILD) -o $(LOCAL_DIR)/$$w ./workloads/$$w || exit 1; \
	done

# Outils qui tournent sur la machine de contrôle (jamais sur les VM).
tools:
	@mkdir -p bin
	@echo "build bin/orchestrator"
	@go build $(GOFLAGS_BUILD) -o bin/orchestrator ./orchestrator
	@echo "build bin/scheduler"
	@go build $(GOFLAGS_BUILD) -o bin/scheduler ./scheduler/cmd/scheduler
	@echo "build bin/gentasks"
	@go build $(GOFLAGS_BUILD) -o bin/gentasks ./scheduler/cmd/gentasks
	@echo "build bin/gantt"
	@go build $(GOFLAGS_BUILD) -o bin/gantt ./scheduler/cmd/gantt

linux:
	@mkdir -p $(LINUX_DIR)
	@for w in $(WORKLOADS); do \
		echo "build $(LINUX_DIR)/$$w"; \
		CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(GOFLAGS_BUILD) -o $(LINUX_DIR)/$$w ./workloads/$$w || exit 1; \
	done

all: local linux

vet:
	go vet ./...

test:
	go test ./...

clean:
	rm -rf bin/
