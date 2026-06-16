AMF ?= 10.100.200.30:38412
COMPOSE ?= docker compose
WEBUI_URL ?= http://127.0.0.1:5050

.PHONY: build config demo-mock demo-proxy demo-ue seed-packetrusher-subscriber down logs ps

build:
	$(COMPOSE) build cgw

config:
	$(COMPOSE) config

demo-mock:
	$(COMPOSE) up --build --force-recreate

demo-proxy:
	CGW_AMF_ADDR=$(AMF) $(COMPOSE) up --build --force-recreate

demo-ue:
	CGW_AMF_ADDR=$(AMF) PACKETRUSHER_CMD="ue --disableTunnel" $(COMPOSE) up --build --force-recreate

seed-packetrusher-subscriber:
	WEBUI_URL=$(WEBUI_URL) ./scripts/seed-packetrusher-subscriber.sh

down:
	$(COMPOSE) down

logs:
	$(COMPOSE) logs -f cgw packetrusher

ps:
	$(COMPOSE) ps
