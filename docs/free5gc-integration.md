# free5GC Integration Notes

This project treats free5GC as an external dependency during development. The CGW repository keeps its own PacketRusher + CGW Compose file small, while a separate free5GC Docker Compose stack joins the same Docker network.

## Target Topology

```text
+----------------+        +----------------+        +----------------+
| PacketRusher   | N2     | 5G-AGW CGW     | N2     | free5GC AMF    |
| 10.100.200.20  | -----> | 10.100.200.10  | -----> | 10.100.200.30  |
+----------------+        +----------------+        +----------------+
       gNB                  transparent proxy              5GC
```

The shared Docker network is:

```text
sbi_network = 10.100.200.0/24
```

## Why Run free5GC Externally?

Keeping free5GC outside this repository avoids mixing two large concerns:

- this repository: CGW behavior, NGAP proxying, ID mapping
- free5GC repository: 5GC deployment, NF configuration, database state

The CGW can still run without free5GC in mock mode, which keeps smoke tests fast.

## 1. Start the CGW Network

From this repository, create the shared network by starting the normal stack once:

```bash
docker compose up --build --force-recreate
```

You can stop it with `Ctrl+C` after the network has been created.

The network is explicitly named `sbi_network`, so an external free5GC Compose stack can attach to it by name.

## 2. Attach free5GC to `sbi_network`

An example override is provided at:

```text
examples/free5gc-sbi-network.override.yml
```

From your free5GC Docker Compose directory, run something like:

```bash
docker compose \
  -f docker-compose.yaml \
  -f /absolute/path/to/5g-agw/examples/free5gc-sbi-network.override.yml \
  up -d
```

The current override targets the official `free5gc-compose` service name `free5gc-amf` and maps its logical `privnet` network onto the external Docker network named `sbi_network`.

## 3. Configure the AMF N2 Address

The AMF container must listen for NGAP/SCTP on the address assigned on `sbi_network`:

```text
10.100.200.30:38412
```

In the current `free5gc-compose` layout, the AMF config is:

```text
config/amfcfg.yaml
```

The default config uses `amf.free5gc.org` in `ngapIpList`, and the override keeps that alias on `10.100.200.30`. If you replace aliases with literal IP addresses, set `ngapIpList` to `10.100.200.30`.

Also make sure the AMF supports the same test values used by PacketRusher:

| Field | Value |
| --- | --- |
| MCC | `999` |
| MNC | `02` |
| TAC | `000001` |
| SST | `01` |
| SD | `000001` |

If these values do not match, the first visible failure will often be `NGSetupFailure` or a later UE registration rejection. For the first transparent NGSetup test, matching `config/amfcfg.yaml` is the critical piece.

## 4. Seed the PacketRusher Subscriber

UE registration requires subscriber data in free5GC's MongoDB. The easiest reproducible path is to run the free5GC WebUI API and seed the PacketRusher UE.

If your host already uses port `5000`, run WebUI on another port. For example:

```bash
docker run -d --name webui5050 \
  --network sbi_network \
  -p 5050:5000 \
  -v /absolute/path/to/free5gc-compose/config/webuicfg.yaml:/free5gc/config/webuicfg.yaml \
  free5gc/webui:v4.2.2 \
  ./webui -c ./config/webuicfg.yaml
```

Then seed the default PacketRusher subscriber from this repository:

```bash
make seed-packetrusher-subscriber WEBUI_URL=http://127.0.0.1:5050
```

The default seeded values are:

| Field | Value |
| --- | --- |
| SUPI | `imsi-999020000000001` |
| PLMN | `99902` |
| Key | `465B5CE8B199B49FAA5F0A2EE238A6BC` |
| OPc | `E8ED289DEBA952E4283B54E88E6183CA` |
| S-NSSAI | `sst=1`, `sd=000001` |
| DNN | `internet` |

## 5. Run CGW in Transparent Proxy Mode

After free5GC AMF is reachable at `10.100.200.30:38412`, run this repository's stack with `CGW_AMF_ADDR` set:

```bash
make demo-proxy
```

Expected CGW logs for the first milestone:

```text
[CGW] Running in transparent proxy mode. Upstream AMF: 10.100.200.30:38412
[CGW] AMFへのSCTP接続を確立: 10.100.200.30:38412
[CGW] gNB -> AMF: pdu=InitiatingMessage procedure=NGSetup procedureCode=21 size=62 bytes
[CGW] AMF -> gNB: pdu=SuccessfulOutcome procedure=NGSetup procedureCode=21 size=... bytes
```

`ProcedureCode=21` is `NGSetup`.

## 6. UE Registration Observation

With AMF, AUSF, UDM, UDR, NSSF, and PCF running, use PacketRusher UE mode:

```bash
make demo-ue
```

Expected CGW log excerpts after subscriber seeding:

```text
[CGW] gNB -> AMF: pdu=InitiatingMessage procedure=InitialUEMessage procedureCode=15 size=70 bytes ranUeNgapId=1
[CGW] AMF -> gNB: pdu=InitiatingMessage procedure=DownlinkNASTransport procedureCode=4 size=66 bytes amfUeNgapId=... ranUeNgapId=1
[CGW] gNB -> AMF: pdu=InitiatingMessage procedure=UplinkNASTransport procedureCode=46 size=... bytes amfUeNgapId=... ranUeNgapId=1
[CGW] AMF -> gNB: pdu=InitiatingMessage procedure=InitialContextSetup procedureCode=14 size=... bytes amfUeNgapId=... ranUeNgapId=1
[CGW] gNB -> AMF: pdu=SuccessfulOutcome procedure=InitialContextSetup procedureCode=14 size=... bytes amfUeNgapId=... ranUeNgapId=1
```

PacketRusher should reach:

```text
[UE][NAS] Receive Registration Accept
```

If it reaches PDU session establishment and then fails with `DNN not supported or not subscribed in the slice`, debug SMF/UPF and subscriber DNN/S-NSSAI data next. This is beyond the first UE registration observation milestone.

## 7. Debugging Checklist

If CGW cannot connect to the AMF:

```bash
docker network inspect sbi_network
```

Check that the AMF container is attached and has `10.100.200.30`.

If SCTP connects but NG setup fails:

- Confirm AMF NGAP/N2 bind address is `10.100.200.30`.
- Confirm PLMN/TAC/S-NSSAI match PacketRusher.
- Check AMF logs for `NGSetupFailure` cause values.
- Check CGW logs for the NGAP direction and procedure code.

If NG setup succeeds but UE registration fails:

- Confirm the PacketRusher subscriber exists in free5GC WebUI.
- Check UDR logs for `authentication-subscription` 404 responses.
- Confirm AUSF, UDM, UDR, NSSF, and PCF are registered in NRF.
- Remove invalid `*-callback` services from free5GC v4.2.2 configs if a NF refuses to start.

## 8. PDU Session / TEID Mapping Verification

After UE registration works, the next C-Plane validation step is to confirm that the CGW observes `PDU Session Resource Setup` and records the GTP-U tunnel endpoint information needed by the future U-Plane gateway.

See:

```text
docs/pdu-session-map-verification.md
```
