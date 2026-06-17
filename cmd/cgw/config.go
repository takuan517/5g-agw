package main

import (
	"os"
	"strings"
)

type config struct {
	ListenAddr string
	AMFAddr    string
}

func loadConfig() config {
	return config{
		ListenAddr: envOrDefault("CGW_LISTEN_ADDR", "0.0.0.0:38412"),
		AMFAddr:    strings.TrimSpace(os.Getenv("CGW_AMF_ADDR")),
	}
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
