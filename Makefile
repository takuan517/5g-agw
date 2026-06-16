AMF ?= 10.100.200.30:38412
COMPOSE ?= docker compose

.PHONY: build config demo-mock demo-proxy down logs ps

build:
	$(COMPOSE) build cgw

config:
	$(COMPOSE) config

demo-mock:
	$(COMPOSE) up --build --force-recreate

demo-proxy:
	CGW_AMF_ADDR=$(AMF) $(COMPOSE) up --build --force-recreate

down:
	$(COMPOSE) down

logs:
	$(COMPOSE) logs -f cgw packetrusher

ps:
	$(COMPOSE) ps
