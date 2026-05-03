PLATFORMS ?= linux/amd64,linux/arm64

VERSION := $(shell git rev-parse --short HEAD 2>/dev/null || echo "dev")

UNAME_M := $(shell uname -m)
LOCAL_PLATFORM := $(if $(filter $(UNAME_M),x86_64),linux/amd64,$(if $(filter $(UNAME_M),aarch64 arm64),linux/arm64,linux/amd64))

DF_VIDEO := containers/images/Dockerfile
DF_TURN  := containers/images/Dockerfile.turn

IMAGE_VIDEO := gonnekone/videoapp
IMAGE_TURN  := gonnekone/turn

.PHONY: buildx-init build-dev push push-turn deploy \
        clean-dev run-dev down-dev logs-dev \
        stop-prod clean-prod run-prod logs-prod version

version:
	@echo "Current version: $(VERSION)"

buildx-init:
	@docker buildx create --name mb --use 2>/dev/null || docker buildx use mb
	@docker run --privileged --rm tonistiigi/binfmt --install all >/dev/null
	@docker buildx inspect --bootstrap >/dev/null
	@echo "buildx готов (builder=mb), binfmt установлен"

build-dev: buildx-init
	docker buildx build --load --platform $(LOCAL_PLATFORM) \
		-t $(IMAGE_VIDEO):dev -f $(DF_VIDEO) .
	docker buildx build --load --platform $(LOCAL_PLATFORM) \
		-t $(IMAGE_TURN):dev  -f $(DF_TURN)  .

push: buildx-init
	@echo ">>> Pushing $(IMAGE_VIDEO):$(VERSION)"
	@git lfs pull 2>/dev/null || true
	docker buildx build --no-cache --platform $(PLATFORMS) \
		--build-arg VERSION=$(VERSION) \
		-t $(IMAGE_VIDEO):$(VERSION) \
		-t $(IMAGE_VIDEO):latest \
		-f $(DF_VIDEO) --push .
	@echo "<<< Done: $(IMAGE_VIDEO):$(VERSION)"

push-turn: buildx-init
	@echo ">>> Pushing $(IMAGE_TURN):$(VERSION)"
	docker buildx build --no-cache --platform $(PLATFORMS) \
		-t $(IMAGE_TURN):$(VERSION) \
		-t $(IMAGE_TURN):latest \
		-f $(DF_TURN) --push .
	@echo "<<< Done: $(IMAGE_TURN):$(VERSION)"

# для деплоя на сервере:
# 1 — сохраняем текущие образы как backup
# sudo docker tag gonnekone/videoapp:latest gonnekone/videoapp:backup
# sudo docker tag gonnekone/turn:latest gonnekone/turn:backup

# 2 — подтягиваем новые образы
# sudo docker pull gonnekone/videoapp:latest && sudo docker pull gonnekone/turn:latest

# 3 — перезапускаем только app и turn, caddy не трогаем
# sudo docker compose up -d --no-deps app turn

# посмотреть бекапы
# sudo docker images | grep gonnekone
deploy: push push-turn
	@echo ""
	@echo "==============================="
	@echo " Deployed version: $(VERSION)"
	@echo "==============================="
	@echo " $(IMAGE_VIDEO):$(VERSION)"
	@echo " $(IMAGE_TURN):$(VERSION)"
	@echo ""
	@echo " Чтобы запустить эту версию на сервере:"
	@echo " Обновите dc.prod.yml:"
	@echo "   image: $(IMAGE_VIDEO):$(VERSION)"
	@echo "   image: $(IMAGE_TURN):$(VERSION)"
	@echo ""
	@echo " Затем: make run-prod"

run-dev:
	docker compose -f containers/composes/dc.dev.yml up

down-dev:
	docker compose -f containers/composes/dc.dev.yml down

clean-dev:
	docker compose -f containers/composes/dc.dev.yml down --volumes

logs-dev:
	docker compose -f containers/composes/dc.dev.yml logs -f --tail 100

stop-prod:
	docker compose -f containers/composes/dc.prod.yml stop --timeout 120

clean-prod:
	docker compose -f containers/composes/dc.prod.yml down --timeout 120

run-prod:
	docker compose -f containers/composes/dc.prod.yml up -d --timeout 120

logs-prod:
	docker compose -f containers/composes/dc.prod.yml logs -f --tail 100