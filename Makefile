PLATFORMS ?= linux/amd64,linux/arm64

UNAME_M := $(shell uname -m)
LOCAL_PLATFORM := $(if $(filter $(UNAME_M),x86_64),linux/amd64,$(if $(filter $(UNAME_M),aarch64 arm64),linux/arm64,linux/amd64))

DF_VIDEO := containers/images/Dockerfile
DF_TURN  := containers/images/Dockerfile.turn

.PHONY: buildx-init build-dev push-dev push-turn clean-dev run-dev down-dev logs-dev stop-prod clean-prod run-prod logs-prod

# Инициализация buildx и binfmt (выполнить один раз на машине)
buildx-init:
	@docker buildx create --name mb --use 2>/dev/null || docker buildx use mb
	@docker run --privileged --rm tonistiigi/binfmt --install all >/dev/null
	@docker buildx inspect --bootstrap >/dev/null
	@echo "buildx готов (builder=mb), binfmt установлен"

# DEV

build-dev: buildx-init
	docker buildx build --load --platform $(LOCAL_PLATFORM) -t videoapp -f $(DF_VIDEO) .
	docker buildx build --load --platform $(LOCAL_PLATFORM) -t turn     -f $(DF_TURN)  .

push-dev: buildx-init
	docker buildx build --platform $(PLATFORMS) -t gonnekone/videoapp:latest -f $(DF_VIDEO) --push .

push-turn: buildx-init
	docker buildx build --platform $(PLATFORMS) -t gonnekone/turn:latest -f $(DF_TURN) --push .

clean-dev:
	docker compose -f containers/composes/dc.dev.yml down

run-dev:
	docker compose -f containers/composes/dc.dev.yml up

down-dev:
	docker compose -f containers/composes/dc.dev.yml down

logs-dev:
	docker compose -f containers/composes/dc.dev.yml logs -f --tail 100

# PROD

stop-prod:
	docker compose -f containers/composes/dc.prod.yml stop --timeout 120

clean-prod:
	docker compose -f containers/composes/dc.prod.yml down --timeout 120

run-prod:
	docker compose -f containers/composes/dc.prod.yml up -d --timeout 120

logs-prod:
	docker compose -f containers/composes/dc.prod.yml logs -f --tail 100