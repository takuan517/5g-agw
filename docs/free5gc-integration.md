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
  -f docker-compose.yml \
  -f /absolute/path/to/5g-agw/examples/free5gc-sbi-network.override.yml \
  up -d
```

If your free5GC Compose service is not named `amf`, edit the override file and replace `amf` with the actual AMF service name.

## 3. Configure the AMF N2 Address

The AMF container must listen for NGAP/SCTP on the address assigned on `sbi_network`:

```text
10.100.200.30:38412
```

In free5GC, this usually means updating the AMF configuration so its NGAP/N2 bind address includes `10.100.200.30`. The exact file path depends on the free5GC Docker setup, but it is commonly an AMF config file mounted into the AMF container.

Also make sure the AMF supports the same test values used by PacketRusher:

| Field | Value |
| --- | --- |
| MCC | `999` |
| MNC | `02` |
| TAC | `000001` |
| SST | `01` |
| SD | `000001` |

If these values do not match, the first visible failure will often be `NGSetupFailure` or a later UE registration rejection.

## 4. Run CGW in Transparent Proxy Mode

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

## 5. Debugging Checklist

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

If NG setup succeeds but UE registration fails, that is expected for the next phase. UE registration needs the rest of the 5GC services and will introduce NAS/security/session behavior beyond the first transparent NGSetup milestone.
