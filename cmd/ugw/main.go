package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"

	"5g-agw/internal/uplane"
)

func main() {
	config, err := loadConfig()
	if err != nil {
		log.Fatalf("[UGW] config error: %v", err)
	}

	router := uplane.NewRouter()
	if config.StaticRoute != nil {
		router.Upsert(*config.StaticRoute)
		log.Printf("[UGW] static route loaded: session=%s ran=%s/0x%08x core=%s/0x%08x",
			config.StaticRoute.SessionID,
			config.StaticRoute.RAN.Addr,
			config.StaticRoute.RAN.TEID,
			config.StaticRoute.Core.Addr,
			config.StaticRoute.Core.TEID,
		)
	} else {
		log.Printf("[UGW] no static route configured; packets will be dropped until routes are installed")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	gateway := uplane.NewUDPGateway(uplane.UDPGatewayConfig{
		RANListen:  config.RANListen,
		CoreListen: config.CoreListen,
	}, router, log.Default())

	if err := gateway.Serve(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("[UGW] stopped with error: %v", err)
	}
}
