# PDU Session Mapping Verification Guide

This guide explains how to verify that the CGW observes `PDU Session Resource Setup` messages and stores the observed GTP-U tunnel information in the in-memory PDU Session mapping table.

The goal is to see `[CGW][PDU]` and `[CGW][PDU-MAP]` logs containing:

- `PDU Session ID`
- UL/DL tunnel direction
- GTP-U transport address
- TEID

## Target Topology

```text
----------------+        N2/SCTP        +-------------+        N2/SCTP        +-------------+
| PacketRusher   | -------------------> | 5G-AGW CGW  | -------------------> | free5GC AMF |
| 10.100.200.20  |                      | 10.100.200.10|                      | 10.100.200.30|
+----------------+                      +-------------+                      +-------------+

                                   N3/GTP-U is not proxied yet
```

The current CGW still runs only as a C-Plane proxy. This verification checks that the C-Plane has enough information to prepare for the future U-Plane gateway.

## Prerequisites

- Docker and Docker Compose are available.
- free5GC is running on the shared Docker network `sbi_network`.
- free5GC AMF is reachable at `10.100.200.30:38412`.
- PacketRusher subscriber data has been seeded into free5GC.
- The CGW image has been rebuilt after the latest code changes.

For the free5GC setup flow, see:

```text
docs/free5gc-integration.md
```

## 1. Start free5GC

From your free5GC Compose directory, start free5GC with the override that attaches it to `sbi_network`.

Example:

```bash
docker compose \
  -f docker-compose.yaml \
  -f /absolute/path/to/5g-agw/examples/free5gc-sbi-network.override.yml \
  up -d
```

Confirm the AMF is attached to `sbi_network`:

```bash
docker network inspect sbi_network
```

Expected:

```text
free5gc-amf -> 10.100.200.30
```

## 2. Seed the PacketRusher Subscriber

If the subscriber is not already registered in free5GC, start the WebUI API and seed the default PacketRusher UE.

Example WebUI container:

```bash
docker run -d --name webui5050 \
  --network sbi_network \
  -p 5050:5000 \
  -v /absolute/path/to/free5gc-compose/config/webuicfg.yaml:/free5gc/config/webuicfg.yaml \
  free5gc/webui:v4.2.2 \
  ./webui -c ./config/webuicfg.yaml
```

From this repository:

```bash
make seed-packetrusher-subscriber WEBUI_URL=http://127.0.0.1:5050
```

The seeded UE should match PacketRusher's default test values:

| Field | Value |
| --- | --- |
| SUPI | `imsi-999020000000001` |
| PLMN | `99902` |
| S-NSSAI | `sst=1`, `sd=000001` |
| DNN | `internet` |

## 3. Build CGW

From this repository:

```bash
make build
```

This should build the `5g-agw-cgw` image successfully.

## 4. Run UE Mode Through the Transparent Proxy

Run PacketRusher UE mode with CGW connected to the free5GC AMF:

```bash
make demo-ue
```

This expands to:

```bash
CGW_AMF_ADDR=10.100.200.30:38412 \
PACKETRUSHER_CMD="ue --disableTunnel" \
docker compose up --build --force-recreate
```

`--disableTunnel` keeps PacketRusher in C-Plane oriented mode, so a local `gtp5g` kernel module is not required.

## 5. Confirm Baseline C-Plane Success

First confirm that NG setup and UE registration still pass.

Expected CGW logs:

```text
[CGW] Running in transparent proxy mode. Upstream AMF: 10.100.200.30:38412
[CGW] AMFへのSCTP接続を確立: 10.100.200.30:38412
[CGW] gNB -> AMF: pdu=InitiatingMessage procedure=NGSetup procedureCode=21
[CGW] AMF -> gNB: pdu=SuccessfulOutcome procedure=NGSetup procedureCode=21
[CGW] gNB -> AMF: pdu=InitiatingMessage procedure=InitialUEMessage procedureCode=15
```

Expected PacketRusher log:

```text
[UE][NAS] Receive Registration Accept
```

If registration does not reach `Registration Accept`, debug subscriber data and free5GC NF status before checking PDU Session mapping.

## 6. Check PDU Session Observation Logs

When the UE attempts PDU Session establishment, look for `[CGW][PDU]` logs.

Expected shape:

```text
[CGW][PDU] AMF -> gNB: procedure=PDUSessionResourceSetup pduSessionId=1 tunnel=UL addr=<upf-or-core-side-address> teid=<number> teidHex=0x<hex>
[CGW][PDU] gNB -> AMF: procedure=PDUSessionResourceSetup pduSessionId=1 tunnel=DL addr=<gnb-side-address> teid=<number> teidHex=0x<hex>
```

The exact address and TEID values depend on free5GC and PacketRusher runtime state.

## 7. Check PDU Session Mapping Logs

After `[CGW][PDU]` appears, the mapping table should also log `[CGW][PDU-MAP]`.

Expected shape:

```text
[CGW][PDU-MAP] assoc=1 direction=AMF -> gNB procedure=PDUSessionResourceSetup pduSessionId=1 originalRanUeNgapId=1 gatewayRanUeNgapId=1000000 amfUeNgapId=<id> ul=<addr>/<teid> dl=<pending>
[CGW][PDU-MAP] assoc=1 direction=gNB -> AMF procedure=PDUSessionResourceSetup pduSessionId=1 originalRanUeNgapId=1 gatewayRanUeNgapId=1000000 amfUeNgapId=<id> ul=<addr>/<teid> dl=<addr>/<teid>
```

The important part is that one mapping eventually contains both:

- `ul=<addr>/0x...`
- `dl=<addr>/0x...`

That means the C-Plane has captured both sides of the GTP-U tunnel information needed by the future U-Plane gateway.

## 8. Known Current Limitation

The current CGW does not proxy N3/GTP-U yet.

So this verification is successful even if PacketRusher later reports a PDU Session or DNN-related failure, as long as the CGW logs show the PDU Session setup attempt and the tunnel information.

One known free5GC-side failure is:

```text
DNN not supported or not subscribed in the slice
```

If that happens before `PDUSessionResourceSetup`, then free5GC rejected the PDU Session before tunnel setup. Check subscriber DNN/S-NSSAI data, SMF, UPF, and slice configuration.

## 9. Useful Commands

Follow CGW and PacketRusher logs:

```bash
make logs
```

Show running services:

```bash
make ps
```

Stop the CGW and PacketRusher stack:

```bash
make down
```

Check the shared Docker network:

```bash
docker network inspect sbi_network
```

## 10. Troubleshooting

If `[CGW][PDU]` does not appear:

- Confirm UE registration reaches `Registration Accept`.
- Confirm PacketRusher attempts PDU Session establishment after registration.
- Check free5GC SMF/UPF logs.
- Check subscriber DNN and S-NSSAI.
- Check whether free5GC rejects the PDU Session before sending `PDUSessionResourceSetupRequest`.

If `[CGW][PDU]` appears but `[CGW][PDU-MAP]` does not:

- Check whether UE ID rewrite logs appeared earlier.
- Look for `[CGW][PDU-MAP] ... skipped: UE mapping not found`.
- Confirm `InitialUEMessage` was rewritten and a gateway-managed `RAN UE NGAP ID` was allocated.

If only one of `ul` or `dl` is present:

- That can be normal during the middle of the setup flow.
- Wait until both AMF-to-gNB and gNB-to-AMF `PDUSessionResourceSetup` messages pass.
- If the flow ends early, check PacketRusher and free5GC logs for the rejection cause.
