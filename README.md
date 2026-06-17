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

The project currently implements the first proxy milestone:

- Starts an SCTP server on `0.0.0.0:38412`.
- Accepts an SCTP association from PacketRusher acting as a gNB.
- Decodes and logs NGAP direction, PDU type, procedure name, procedure code, payload size, and UE IDs where present.
- Runs in mock AMF mode and builds/sends `NGSetupResponse` directly from the CGW.
- Runs in transparent proxy mode and relays NGAP between PacketRusher and a real free5GC AMF.
- Confirms transparent `NGSetupRequest -> NGSetupResponse` through free5GC AMF.
- Confirms UE registration traffic through `InitialUEMessage`, authentication, security mode, `InitialContextSetup`, and `Registration Accept`.
- Maintains a read-only UE mapping table that observes `RAN UE NGAP ID <-> AMF UE NGAP ID` relationships per SCTP association.
- Rewrites `RAN UE NGAP ID` for UE-associated NGAP messages using a gateway-managed ID. The first gateway-managed ID is allocated when `InitialUEMessage` is received, then restored back to the original gNB-side ID on the downstream path.

The next engineering milestone is to harden this rewrite layer for multiple gNBs/UEs and broaden coverage beyond the first validated UE registration flow.

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
├── cmd/cgw/                    # CGW entrypoint, proxy, NGAP logging, and mapping
├── config/packetrusher.yaml    # PacketRusher gNB/UE test configuration
├── docs/free5gc-integration.md # External free5GC integration guide
├── examples/                   # Compose override examples
├── internal/context/nat.go     # Early NAT/UE context model
├── Dockerfile                  # CGW container image
├── Dockerfile.pr               # PacketRusher container image
├── docker-compose.yml          # Local test topology
├── Makefile                    # Common development commands
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
make demo-mock
```

Expected CGW log excerpts:

```text
[CGW] Listening for gNB on 0.0.0.0:38412...
[CGW] gNBからの新規接続を受信: 10.100.200.20:9487
[CGW] 62 バイトのSCTPデータを受信
[CGW] gNB -> CGW: pdu=InitiatingMessage procedure=NGSetup procedureCode=21 size=62 bytes
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

## Runtime Modes

The CGW can run in two modes.

### Mock AMF Mode

This is the default mode. If `CGW_AMF_ADDR` is not set, the CGW responds to `NGSetupRequest` by itself.

Use this mode for fast C-Plane smoke tests when no real 5GC is running.

```bash
make demo-mock
```

### Transparent Proxy Mode

If `CGW_AMF_ADDR` is set, the CGW opens a northbound SCTP connection to that AMF and forwards NGAP messages between the gNB and the AMF.

```bash
make demo-proxy
```

To use a different AMF address:

```bash
make demo-proxy AMF=10.100.200.31:38412
```

In this mode, an AMF should be running and reachable from the CGW container. For development, it is best to run a full test 5GC stack, such as free5GC or Open5GS, on the same Docker network and configure its PLMN/TAC/S-NSSAI values to match PacketRusher.

For the recommended external free5GC workflow, see [`docs/free5gc-integration.md`](docs/free5gc-integration.md).

The current PacketRusher test values are:

| Field | Value |
| --- | --- |
| MCC | `999` |
| MNC | `02` |
| TAC | `000001` |
| SST | `01` |
| SD | `000001` |

For early transparent proxy validation, the first target is only `NGSetupRequest -> AMF -> NGSetupResponse`. Full UE registration will require additional 5GC components beyond AMF.

Expected CGW proxy logs for a successful NG setup relay:

```text
[CGW] Running in transparent proxy mode. Upstream AMF: 10.100.200.30:38412
[CGW] AMFへのSCTP接続を確立: 10.100.200.30:38412
[CGW] gNB -> AMF: pdu=InitiatingMessage procedure=NGSetup procedureCode=21 size=62 bytes
[CGW] AMF -> gNB: pdu=SuccessfulOutcome procedure=NGSetup procedureCode=21 size=... bytes
```

## Development Commands

Common commands are wrapped by `make`:

| Command | Description |
| --- | --- |
| `make build` | Build the CGW Docker image |
| `make config` | Render the Docker Compose configuration |
| `make demo-mock` | Run CGW + PacketRusher with CGW mock AMF responses |
| `make demo-proxy` | Run CGW + PacketRusher with `CGW_AMF_ADDR=10.100.200.30:38412` |
| `make demo-ue` | Run CGW + PacketRusher `ue --disableTunnel` for UE message observation |
| `make demo-proxy AMF=IP:PORT` | Run transparent proxy mode against a custom AMF |
| `make seed-packetrusher-subscriber` | Seed the PacketRusher UE subscriber through free5GC WebUI API |
| `make logs` | Follow CGW and PacketRusher logs |
| `make ps` | Show Compose service status |
| `make down` | Stop and remove the Compose stack |

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
5. Observe RAN UE NGAP ID and AMF UE NGAP ID mapping.
6. Rewrite NGAP UE identifiers using the mapping table.
7. Harden the rewrite path for multiple UE-associated procedures.
8. Support multiple gNB SCTP associations aggregated through the CGW.
9. Add structured logs, metrics, and integration test scripts.

## Development Philosophy

This repository is intentionally small and direct while the NGAP behavior is being explored. The current priority is validating protocol behavior end-to-end before introducing heavier abstractions.
