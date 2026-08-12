package main

import (
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"

	"5g-agw/internal/uplane"
)

type config struct {
	RANListen   netip.AddrPort
	CoreListen  netip.AddrPort
	StaticRoute *uplane.TunnelRoute
}

func loadConfig() (config, error) {
	ranListen, err := netip.ParseAddrPort(envOrDefault("UGW_RAN_LISTEN_ADDR", "0.0.0.0:2152"))
	if err != nil {
		return config{}, fmt.Errorf("UGW_RAN_LISTEN_ADDR: %w", err)
	}
	coreListen, err := netip.ParseAddrPort(envOrDefault("UGW_CORE_LISTEN_ADDR", "0.0.0.0:2153"))
	if err != nil {
		return config{}, fmt.Errorf("UGW_CORE_LISTEN_ADDR: %w", err)
	}

	var staticRoute *uplane.TunnelRoute
	if raw := os.Getenv("UGW_STATIC_ROUTE"); raw != "" {
		route, err := parseStaticRoute(raw)
		if err != nil {
			return config{}, fmt.Errorf("UGW_STATIC_ROUTE: %w", err)
		}
		staticRoute = &route
	}

	return config{
		RANListen:   ranListen,
		CoreListen:  coreListen,
		StaticRoute: staticRoute,
	}, nil
}

func parseStaticRoute(raw string) (uplane.TunnelRoute, error) {
	parts := strings.Split(raw, ",")
	if len(parts) != 5 {
		return uplane.TunnelRoute{}, fmt.Errorf("expected sessionID,ranAddr,ranTEID,coreAddr,coreTEID")
	}

	ranAddr, err := netip.ParseAddrPort(strings.TrimSpace(parts[1]))
	if err != nil {
		return uplane.TunnelRoute{}, fmt.Errorf("RAN address: %w", err)
	}
	ranTEID, err := parseTEID(parts[2])
	if err != nil {
		return uplane.TunnelRoute{}, fmt.Errorf("RAN TEID: %w", err)
	}
	coreAddr, err := netip.ParseAddrPort(strings.TrimSpace(parts[3]))
	if err != nil {
		return uplane.TunnelRoute{}, fmt.Errorf("core address: %w", err)
	}
	coreTEID, err := parseTEID(parts[4])
	if err != nil {
		return uplane.TunnelRoute{}, fmt.Errorf("core TEID: %w", err)
	}

	return uplane.TunnelRoute{
		SessionID: strings.TrimSpace(parts[0]),
		RAN:       uplane.Endpoint{Addr: ranAddr, TEID: ranTEID},
		Core:      uplane.Endpoint{Addr: coreAddr, TEID: coreTEID},
	}, nil
}

func parseTEID(raw string) (uint32, error) {
	value, err := strconv.ParseUint(strings.TrimSpace(raw), 0, 32)
	if err != nil {
		return 0, err
	}
	return uint32(value), nil
}

func envOrDefault(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
