# 5G-AGW

5G-AGW is an experimental 5G Access Gateway project written in Go.

The current focus is a C-Plane Gateway (CGW) that sits between simulated gNBs and a 5G Core AMF on the N2 interface. The long-term goal is to implement an NGAP back-to-back proxy that can aggregate multiple gNB/UE connections and present them upstream as a controlled, gateway-managed view, similar in spirit to NAT for NGAP identifiers.

## Project Goals

- Accept SCTP/NGAP connections from one or more gNBs.
- Decode NGAP messages received on the N2 interface.
- Act as an AMF-facing endpoint for early C-Plane validation.
- Later, proxy NGAP traffic transparently between gNBs and a real AMF.
- Eventually implement ID mapping for multiple gNBs/UEs, including RAN UE NGAP ID and AMF UE NGAP ID translation.

## Current Status

The project currently implements the first standalone CGW milestone:

- Starts an SCTP server on `0.0.0.0:38412`.
- Accepts an SCTP association from PacketRusher acting as a gNB.
- Decodes the incoming NGAP `NGSetupRequest`.
- Builds and sends an `NGSetupResponse` directly from the CGW.
- Brings PacketRusher's gNB-side AMF state to `Active`.

This means the CGW can currently pretend to be a minimal AMF for NG setup validation.

## Architecture

```text
+----------------+        SCTP / NGAP N2        +----------------+
| PacketRusher   | ---------------------------> | 5G-AGW CGW     |
| simulated gNB  |                              | mock AMF side  |
| 10.100.200.20  | <--------------------------- | 10.100.200.10  |
+----------------+        NGSetupResponse       +----------------+
```

The next major architecture step is to add a northbound SCTP client from CGW to a real AMF:

```text
+-----+        +----------+        +----------+
| gNB | <----> |   CGW    | <----> |   AMF    |
+-----+        +----------+        +----------+
              NGAP B2B proxy
```

## Tech Stack

- Go `1.26.2`
- Docker / Docker Compose
- PacketRusher for gNB/UE emulation
- `github.com/free5gc/sctp` for SCTP
- `github.com/free5gc/ngap` for NGAP encoding/decoding
- `github.com/free5gc/aper` for ASN.1 APER support

## Repository Layout

```text
.
├── cmd/cgw/main.go             # CGW entrypoint and NGSetup handling
├── config/packetrusher.yaml    # PacketRusher gNB/UE test configuration
├── internal/context/nat.go     # Early NAT/UE context model
├── Dockerfile                  # CGW container image
├── Dockerfile.pr               # PacketRusher container image
├── docker-compose.yml          # Local test topology
├── go.mod
└── go.sum
```

## Local Test Topology

Docker Compose creates a bridge network named `sbi_network`:

| Component | IP | Port | Role |
| --- | --- | --- | --- |
| CGW | `10.100.200.10` | `38412/SCTP` | N2 endpoint / mock AMF |
| PacketRusher | `10.100.200.20` | `9487/SCTP` | simulated gNB |

PacketRusher is configured to connect to the CGW as its AMF endpoint.

## Running the Demo

Build and run both containers:

```bash
docker compose up --build --force-recreate
```

Expected CGW log excerpts:

```text
[CGW] Listening for gNB on 0.0.0.0:38412...
[CGW] gNBからの新規接続を受信: 10.100.200.20:9487
[CGW] 62 バイトのSCTPデータを受信
[CGW] -> 基地局からの初期登録リクエスト (NGSetupRequest) を検知しました！
[CGW] NG Setup Response を送信しました！ SCTPリンク確立完了！
```

Expected PacketRusher log excerpts:

```text
[GNB][NGAP] Receive NG Setup Response
[GNB][AMF] AMF Name: 5G-AGW
[GNB][AMF] State of AMF: Active
[GNB] NG Setup successful with N2 IP: 10.100.200.20:9487
```

Stop the demo with `Ctrl+C`.

## NGSetupResponse Handling

The CGW currently builds an `NGSetupResponse` using `free5gc/ngap` and `free5gc/aper`.

The response includes:

- `AMFName`: `5G-AGW`
- `ServedGUAMIList`: PLMN `999-02`
- `RelativeAMFCapacity`: `100`
- `PLMNSupportList`: PLMN `999-02`, S-NSSAI `sst=01`, `sd=000001`

A packet replay escape hatch is also available. If `CGW_NGSETUP_RESPONSE_HEX` is set, the CGW will decode that hex string and send it directly instead of building the NGAP PDU dynamically.

Example known-good response hex:

```text
2015002f00000400010008028035472d41475700600008000099f92001004000564001640050000b0099f92000001008000001
```

## Notes on Local Development

The SCTP dependency is intended to run inside the Linux container. Building the full CGW package directly on macOS may fail because the SCTP implementation depends on Linux SCTP behavior and syscall bindings.

For reliable validation, prefer Docker-based builds:

```bash
docker compose build cgw
```

## Roadmap

1. Add a northbound SCTP client from CGW to a real AMF.
2. Forward `NGSetupRequest` to the AMF and relay `NGSetupResponse` back to the gNB.
3. Generalize the forwarding path for bidirectional NGAP messages.
4. Introduce per-gNB and per-UE context tracking.
5. Implement RAN UE NGAP ID and AMF UE NGAP ID mapping.
6. Support multiple gNB SCTP associations aggregated through the CGW.
7. Add structured logs, metrics, and integration test scripts.

## Development Philosophy

This repository is intentionally small and direct while the NGAP behavior is being explored. The current priority is validating protocol behavior end-to-end before introducing heavier abstractions.
