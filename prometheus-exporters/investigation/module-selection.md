# Module Selection Mapping

How the dc-observability-operator's per-device SNMP polling maps onto snmp_exporter
modules and auths.

## device_type -> snmp_exporter module

The operator resolves a device *family* from `device_types[0]` using ordered match rules
(`roles/agent/defaults/main.yml:36-60`). Each family becomes one snmp_exporter module.

| Order | Operator match rule | snmp_exporter module | MIBs |
|-------|---------------------|----------------------|------|
| 1 | `device_type` **equalto** `panduit-epa126` | `panduit-epa126` | RFC1213-MIB, SNMPv2-MIB, HAWK-I2-MIB |
| 2 | `device_type` **search** `panduit-p` | `panduit-g5` | RFC1213-MIB, SNMPv2-MIB, PANDUIT-MIB |
| 3 | `device_type` **search** `apc-apdu` | `apc-netshelter` | RFC1213-MIB, SNMPv2-MIB, PowerNet-MIB |
| 4 | `device_type` **search** `hwgroup-ste2plus` | `hwgroup-ste2plus` | RFC1213-MIB, SNMPv2-MIB, STE2-PLUS-MIB |
| 5 | `device_type` **search** `apc-netbotz` | `apc-netbotz` | RFC1213-MIB, SNMPv2-MIB, PowerNet-MIB |
| 6 | `device_type` **search** `akcp-sensor` | `akcp-sensorprobe2pro` | RFC1213-MIB, SNMPv2-MIB, SPAGENT-MIB |

Rules are evaluated top-down; `equalto` is exact, `search` is a substring/regex match.
Because `panduit-epa126` matches first (exact), EPA126 gateways do not fall through to
`panduit-p`.

## Test hosts (test-inventory.yaml) -> module + auth

All 7 test hosts are SNMP v2c on 127.0.0.1 (SNMP simulator loopback). The `community`
values are simulator profile names, mapped to one snmp_exporter auth each.

| Host | device_type | module | community / auth |
|------|-------------|--------|------------------|
| DC-ROOM-ROW-01 | panduit-epa126 | `panduit-epa126` | `panduit-gen3` |
| DC-ROOM-ROW-02 | panduit-epa126 | `panduit-epa126` | `panduit-gen3-gen6pdu` |
| DC-ROOM-ROW-03 | panduit-p48fe04 | `panduit-g5` | `panduit-gen5` |
| DC-ROOM-ROW-04 | panduit-p36f50g-32a3wye | `panduit-g5` | `panduit-gen6` |
| DC-ROOM-ROW-05 | hwgroup-ste2plus | `hwgroup-ste2plus` | `hwgroup-ste2plus` |
| DC-ROOM-ROW-06 | apc-netbotz | `apc-netbotz` | `apc-netbotz` |
| DC-ROOM-ROW-07 | akcp-sensorprobe2pro | `akcp-sensorprobe2pro` | `akcp-sensorprobe2pro` |

`apc-netshelter` (match `apc-apdu`) has **no** test host, but the module exists for
production `apc-apdu*` devices.

## Auth mapping (operator -> snmp_exporter)

v2c (all test hosts): `community` -> snmp_exporter auth `community`, `version: 2`.

v3 (supported by operator, unused by test hosts):

| Operator (config_context.dcm.observability.snmp.*) | snmp_exporter auth field |
|-----------------------------------------------------|--------------------------|
| `username` | `username` |
| `security` | `security_level` (noAuthNoPriv / authNoPriv / authPriv) |
| `authProtocol` | `auth_protocol` (SHA / MD5 / ...) |
| `authPassword` | `password` |
| `privProtocol` | `priv_protocol` (AES / DES / ...) |
| `privPassword` | `priv_password` |
| `contextName` | `context_name` |

Operator defaults (`roles/agent/defaults/main.yml:70-80`): version 2, community `public`,
retries 1, sec_level `authNoPriv`. snmp_exporter modules set `retries: 1`, `timeout: 5s`
to mirror the operator (`agent_timeout` default 5s).

## Ports

- **Poll: UDP/161** — operator never sets a port (Telegraf default); snmp_exporter also
  defaults the target to udp/161.
- **Traps: UDP/1162** — operator runs a Telegraf `[[inputs.snmp_trap]]` receiver
  (Service port 162 -> container 1162). snmp_exporter has **no trap equivalent**; SNMP
  traps are out of scope for this rewrite and would need a separate trap sink.
