package main

import (
	"fmt"
	"log"
)

func main() {
	fmt.Println("=========================================")
	fmt.Println("Starting 5G-AGW: C-Plane Gateway (CGW)...")
	fmt.Println("=========================================")

	cfg := loadConfig()
	if cfg.AMFAddr == "" {
		log.Printf("[CGW] Running in mock AMF mode. Set CGW_AMF_ADDR to enable transparent proxy mode.")
	} else {
		log.Printf("[CGW] Running in transparent proxy mode. Upstream AMF: %s", cfg.AMFAddr)
	}

	runServer(cfg)
}
