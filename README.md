# 5G-AGW

5G-AGW is an experimental 5G Access Gateway written in Go. It is designed to sit between one or more 5G RAN nodes and a 5G Core, providing back-to-back proxy functions for both the control plane and the user plane.

The final target is a gateway that can aggregate multiple gNBs and UEs behind a controlled gateway-managed view toward the 5G Core. Conceptually, it behaves like a NAT gateway for 5G identifiers:

- C-Plane: NGAP/N2 identifier translation and message proxying
- U-Plane: GTP-U/N3 TEID translation and packet proxying

## Target Architecture

```text
                         +-------------------+
                         |      5G-AGW       |
                         |                   |
+--------+   N2/SCTP     |  +-------------+  |   N2/SCTP     +-----+
| gNB #1 | <-----------> |  |     CGW     |  | <-----------> | AMF |
+--------+   NGAP        |  | C-Plane GW  |  |   NGAP        +-----+
                         |  +-------------+  |
+--------+   N3/GTP-U    |  +-------------+  |   N3/GTP-U    +-----+
| gNB #N | <-----------> |  |     UGW     |  | <-----------> | UPF |
+--------+   UDP/2152    |  | U-Plane GW  |  |   UDP/2152    +-----+
                         |  +-------------+  |
                         +-------------------+
```

The gateway is intended to support two tightly coupled planes:

| Plane | Interface | Protocol | Gateway role |
| --- | --- | --- | --- |
| C-Plane | N2 | SCTP + NGAP | Back-to-back NGAP proxy with UE/gNB ID translation |
| U-Plane | N3 | UDP + GTP-U | Back-to-back GTP-U proxy with TEID translation |

## Goals

The completed 5G-AGW should provide the following capabilities:

- Accept SCTP/NGAP connections from multiple gNBs.
- Maintain a northbound SCTP/NGAP connection to one or more AMFs.
- Proxy NGAP messages bidirectionally between gNBs and AMFs.
- Rewrite NGAP UE identifiers so multiple downstream gNB/UE contexts can be represented safely upstream.
- Maintain mapping tables for gNB association, UE context, AMF UE context, PDU Session, and GTP-U tunnel state.
- Observe C-Plane PDU Session setup procedures and derive the U-Plane tunnel information needed for GTP-U forwarding.
- Accept GTP-U packets from gNBs and UPFs.
- Rewrite GTP-U TEIDs using gateway-managed TEIDs.
- Forward user-plane packets between the RAN side and the core side according to C-Plane-derived session state.
- Clean up C-Plane and U-Plane state on UE release, PDU Session release, and transport connection failure.

## C-Plane Gateway Specification

The C-Plane Gateway, or CGW, handles N2 traffic between gNBs and AMFs.

### Southbound behavior

On the gNB-facing side, the CGW:

- Listens for SCTP associations on the NGAP port, normally `38412`.
- Accepts NGAP messages from simulated or real gNBs.
- Decodes NGAP messages for logging, identifier extraction, and mapping updates.
- Tracks each gNB SCTP association separately.

### Northbound behavior

On the AMF-facing side, the CGW:

- Opens an SCTP client connection to an upstream AMF.
- Relays NGAP messages between the gNB side and the AMF side.
- Presents gateway-managed UE identifiers toward the AMF.

The current development topology uses free5GC AMF at:

```text
10.100.200.30:38412
```

### NGAP identifier translation

The CGW owns a gateway-managed `RAN UE NGAP ID` namespace.

For the first UE message from a gNB:

```text
gNB original RAN UE NGAP ID -> gateway-managed RAN UE NGAP ID
```

For downstream AMF responses:

```text
gateway-managed RAN UE NGAP ID -> gNB original RAN UE NGAP ID
```

The core C-Plane mapping is:

```text
association ID
+ original RAN UE NGAP ID
+ gateway RAN UE NGAP ID
+ AMF UE NGAP ID
```

This allows the gateway to preserve the gNB-facing UE identity while exposing a gateway-controlled identity upstream.

### PDU Session observation

The CGW also observes PDU Session setup procedures because U-Plane state is negotiated through C-Plane NGAP messages.

The intended PDU Session mapping contains:

```text
association ID
+ original RAN UE NGAP ID
+ gateway RAN UE NGAP ID
+ AMF UE NGAP ID
+ PDU Session ID
+ UL GTP-U endpoint address
+ UL TEID
+ DL GTP-U endpoint address
+ DL TEID
```

This table is the handoff point between the C-Plane and the future U-Plane gateway.

### State cleanup

C-Plane state should be removed when:

- `UEContextReleaseComplete` is observed.
- A gNB SCTP association closes.
- A future PDU Session release procedure removes a PDU Session.
- A future AMF-side association fails and contexts must be invalidated or rebuilt.

## U-Plane Gateway Specification

The U-Plane Gateway, or UGW, will handle N3 GTP-U traffic between gNBs and UPFs.

The UGW is not the primary implemented component yet, but the expected design is as follows.

### Southbound behavior

On the gNB-facing side, the UGW should:

- Listen for GTP-U packets on UDP port `2152`.
- Accept packets from one or more gNB N3 addresses.
- Decode the GTP-U header and extract the incoming TEID.
- Resolve the packet to a PDU Session mapping created by the CGW.

### Northbound behavior

On the UPF-facing side, the UGW should:

- Forward GTP-U packets to the selected UPF N3 address.
- Rewrite the TEID to the gateway-managed or UPF-facing TEID expected for that session.
- Maintain reverse mappings for downlink packets from UPF to gNB.

### TEID translation

The intended U-Plane mapping is similar in spirit to NAT:

```text
RAN-side TEID/address <-> gateway-managed TEID <-> UPF-side TEID/address
```

For uplink packets:

```text
gNB TEID -> gateway/UPF-facing TEID
```

For downlink packets:

```text
UPF TEID -> gateway/gNB-facing TEID
```

The UGW should not guess TEID relationships by observing packets alone. It should use the authoritative PDU Session state derived from C-Plane NGAP procedures.

### U-Plane lifecycle

U-Plane state should be created, updated, and removed based on C-Plane events:

| C-Plane event | U-Plane effect |
| --- | --- |
| PDU Session Resource Setup | Create or update TEID mapping |
| PDU Session Resource Modify | Update TEID mapping |
| PDU Session Resource Release | Remove TEID mapping |
| UE Context Release | Remove all UE-related TEID mappings |
| SCTP association close | Remove all affected mappings |

## Runtime Modes

The gateway supports development modes that make incremental protocol validation easier.

### Mock AMF Mode

If `CGW_AMF_ADDR` is not set, the CGW responds to `NGSetupRequest` directly.

This mode is intended for fast SCTP/NGAP smoke tests without a running 5GC.

```bash
make demo-mock
```

### Transparent Proxy Mode

If `CGW_AMF_ADDR` is set, the CGW connects to that AMF and forwards NGAP bidirectionally.

```bash
make demo-proxy
```

The default AMF address used by the Makefile is:

```text
10.100.200.30:38412
```

### UE Observation Mode

PacketRusher UE mode can be used to validate UE registration and observe PDU Session procedures.

```bash
make demo-ue
```

This runs PacketRusher with `ue --disableTunnel`, which avoids requiring a local `gtp5g` kernel module on the PacketRusher side.

## Development Topology

The local Docker topology uses a shared bridge network:

```text
sbi_network = 10.100.200.0/24
```

| Component | IP | Role |
| --- | --- | --- |
| CGW | `10.100.200.10` | 5G-AGW C-Plane Gateway |
| PacketRusher | `10.100.200.20` | Simulated gNB/UE |
| free5GC AMF | `10.100.200.30` | Upstream AMF |

free5GC is treated as an external dependency during development. See:

```text
docs/free5gc-integration.md
```

For PDU Session / TEID mapping verification, see:

```text
docs/pdu-session-map-verification.md
```

## Current Implementation Status

The current implementation focuses on the C-Plane path and has validated the first end-to-end UE registration milestone.

Implemented and verified:

- SCTP server for gNB-side NGAP.
- SCTP client connection to free5GC AMF.
- Bidirectional transparent NGAP proxying.
- `NGSetupRequest` / `NGSetupResponse` relay.
- UE Registration message relay through PacketRusher and free5GC.
- Gateway-managed `RAN UE NGAP ID` allocation.
- Upstream rewrite of gNB-side `RAN UE NGAP ID`.
- Downstream restoration of the original gNB-side `RAN UE NGAP ID`.
- `RAN UE NGAP ID <-> Gateway RAN UE NGAP ID <-> AMF UE NGAP ID` mapping.
- Mapping cleanup on `UEContextReleaseComplete` and SCTP association close.
- PDU Session setup transfer observation logic.
- In-memory PDU Session mapping table for observed UL/DL GTP-U tunnel endpoints.

Validated result:

```text
PacketRusher + CGW + free5GC AMF -> UE Registration Accept
```

Not yet complete:

- Multiple gNB / multiple UE scale validation.
- AMF-side SCTP association aggregation for multiple gNBs.
- Full NGAP procedure coverage beyond the registration path.
- Handover and advanced mobility procedure support.
- PDU Session release / modify hardening.
- UGW implementation for GTP-U packet forwarding and TEID rewrite.
- End-to-end N3 user-plane traffic validation.

## Repository Layout

```text
.
├── cmd/cgw/                    # CGW entrypoint, NGAP proxy, rewrite, logging, and mapping
├── config/packetrusher.yaml    # PacketRusher gNB/UE test configuration
├── docs/                       # Integration and verification guides
├── examples/                   # Compose override examples
├── internal/context/           # Early shared context/NAT model
├── Dockerfile                  # CGW container image
├── Dockerfile.pr               # PacketRusher container image
├── docker-compose.yml          # Local CGW + PacketRusher topology
├── Makefile                    # Common development commands
├── go.mod
└── go.sum
```

## Development Commands

| Command | Description |
| --- | --- |
| `make build` | Build the CGW Docker image |
| `make config` | Render the Docker Compose configuration |
| `make demo-mock` | Run CGW + PacketRusher with CGW mock AMF responses |
| `make demo-proxy` | Run CGW + PacketRusher against free5GC AMF |
| `make demo-ue` | Run PacketRusher UE mode through the CGW |
| `make demo-proxy AMF=IP:PORT` | Run transparent proxy mode against a custom AMF |
| `make seed-packetrusher-subscriber` | Seed the PacketRusher UE subscriber through free5GC WebUI API |
| `make logs` | Follow CGW and PacketRusher logs |
| `make ps` | Show Compose service status |
| `make down` | Stop and remove the Compose stack |

## Notes on macOS Development

The full CGW package depends on Linux SCTP behavior through `github.com/free5gc/sctp`. For reliable local validation on macOS, run the gateway inside Docker rather than with `go run` directly.

Recommended:

```bash
docker compose build cgw
make demo-ue
```

Avoid relying on direct macOS execution for SCTP behavior:

```bash
go run ./cmd/cgw
```

For future U-Plane validation, a Linux environment with suitable GTP-U support may be required. macOS Docker Desktop is sufficient for the current C-Plane registration validation, but it may not support kernel-level `gtp5g` behavior required by a full UPF data-plane test.

## Technology Stack

- Go `1.26.2`
- Docker / Docker Compose
- SCTP for N2 transport
- NGAP for C-Plane signaling
- GTP-U for the planned U-Plane data path
- PacketRusher for gNB/UE emulation
- free5GC for 5GC integration testing
- `github.com/free5gc/sctp`
- `github.com/free5gc/ngap`
- `github.com/free5gc/aper`

## Roadmap

1. Harden C-Plane rewrite behavior for more UE-associated NGAP procedures.
2. Validate multiple UEs behind one gNB association.
3. Validate multiple gNB associations.
4. Design and implement AMF-side SCTP aggregation.
5. Extend PDU Session mapping for modify and release procedures.
6. Implement UGW UDP/GTP-U listener.
7. Implement TEID rewrite and bidirectional GTP-U forwarding.
8. Couple UGW state creation and cleanup to CGW C-Plane events.
9. Validate end-to-end user-plane traffic with a Linux-based free5GC/UPF environment.
10. Add structured metrics, integration tests, and failure recovery behavior.

## Design Principle

5G-AGW keeps C-Plane and U-Plane behavior explicit and observable. The gateway should not hide protocol state behind opaque forwarding. Instead, it should expose the mapping state that makes NGAP and GTP-U translation understandable, testable, and eventually safe to operate with multiple gNBs and UEs.
