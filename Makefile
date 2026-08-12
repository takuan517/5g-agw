AMF ?= 10.100.200.30:38412
COMPOSE ?= docker compose
WEBUI_URL ?= http://127.0.0.1:5050
TEST_GOCACHE ?= $(CURDIR)/.gocache

.PHONY: build build-ugw config demo-mock demo-proxy demo-ue test-internal seed-packetrusher-subscriber down logs ps

build:
	$(COMPOSE) build cgw

build-ugw:
	docker build -f Dockerfile.ugw -t 5g-agw-ugw:latest .

config:
	$(COMPOSE) config

demo-mock:
	$(COMPOSE) up --build --force-recreate

demo-proxy:
	CGW_AMF_ADDR=$(AMF) $(COMPOSE) up --build --force-recreate

demo-ue:
	CGW_AMF_ADDR=$(AMF) PACKETRUSHER_CMD="ue --disableTunnel" $(COMPOSE) up --build --force-recreate

test-internal:
	GOCACHE=$(TEST_GOCACHE) go test ./internal/...

seed-packetrusher-subscriber:
	WEBUI_URL=$(WEBUI_URL) ./scripts/seed-packetrusher-subscriber.sh

down:
	$(COMPOSE) down

logs:
	$(COMPOSE) logs -f cgw packetrusher

ps:
	$(COMPOSE) ps
