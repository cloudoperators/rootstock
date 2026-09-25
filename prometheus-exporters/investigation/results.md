# Results — dc-observability-operator -> Prometheus snmp_exporter rewrite

Research task result summary. All artifacts live under
`$HOME/src/opencode/dc-operator-rewrite/prometheus-snmp/`.

## Task

- Investigate `/Users/I503834/src/github.tools.sap/dc-observability-operator/` (ignore `tmp/`).
- Investigate `/Users/I503834/src/github.com/snmp_exporter`.
- Rewrite the dc-observability-operator in terms of **instantiations of the snmp_exporter**.
- Store all results under `prometheus-snmp/`.

## Agents used

The task named four agents (`researcher`, `researcher-exporter`, `builder`, `tester`). These are
**not defined as opencode subagents** in this environment, so each role was fulfilled with the
available subagents:

| Role | Fulfilled by | Output |
|------|--------------|--------|
| researcher | `explore` subagent | `research/operator-findings.md` |
| researcher-exporter | `explore` subagent | `research/exporter-findings.md` |
| builder | authored directly (subagent hit a mid-run network error) | `generator/generator.yml`, `prometheus/snmp-scrape.yml`, `module-selection.md`, `README.md` |
| tester | validated directly | `research/tester-report.md` |

## Deliverables

```
prometheus-snmp/
  README.md                      rewrite overview, build/run steps, honest gaps
  module-selection.md            device_type match rule -> module; host -> module+auth
  results.md                     this file
  generator/generator.yml        8 auths + 6 snmp_exporter modules (core rewrite)
  prometheus/snmp-scrape.yml     Prometheus scrape config (7 test hosts) + NetBox SD pattern
  research/operator-findings.md  operator SNMP facts (researcher)
  research/exporter-findings.md  snmp_exporter config/generator/runtime schema (researcher-exporter)
  research/tester-report.md      validation results
```

## What the operator does (baseline)

Operator-SDK **Ansible** operator. It does **not** poll SNMP itself: it discovers PDUs /
environmental sensors from **NetBox** (`nb_inventory`), renders **Telegraf** TOML
(`[[inputs.snmp]]` per device family), deploys Telegraf pods that poll over SNMP, and runs a
Telegraf SNMP **trap** receiver. All OIDs are **symbolic** (`MIB::object`); no numeric OIDs exist
in the operator source (only in the ignored `tmp/`).

## The rewrite (what maps to what)

| Operator concept | snmp_exporter equivalent |
|------------------|--------------------------|
| Telegraf `[[inputs.snmp]]` per family | one snmp_exporter **module** per family |
| Per-family symbolic OID lists | module `walk:` (generator resolves via MIBs) |
| Telegraf tag columns (`hw_id`, `hw_name`, ...) | module `lookups:` + `overrides` (`DisplayString`/`EnumAsInfo`) |
| `agent_snmp_community` / v3 params | snmp_exporter **auths** |
| NetBox `nb_inventory` discovery | Prometheus `http_sd`/`file_sd` (adapter) |
| `agent_interval` / `agent_timeout` / `retries` | `scrape_interval` / module `timeout` / `retries` |
| SNMP trap receiver (UDP/1162) | **no equivalent** — out of scope |

## The 6 modules (family -> tables)

| Module | Match rule | MIBs | Tables covered |
|--------|-----------|------|----------------|
| `panduit-epa126` | equalto `panduit-epa126` | RFC1213, SNMPv2, HAWK-I2 | pduTable, pduMonTable, pduMon3PhTable, ipTHATable, ipContTable, idmTable, pduP2BrCctMonitorTable, pduOutTable |
| `panduit-g5` | search `panduit-p` | RFC1213, SNMPv2, PANDUIT | pdug5IdentTable, pdug5InputTable, pdug5InputPhaseTable, pdug5TemperatureTable, pdug5HumidityTable, pdug5GroupTable, pdug5OutletTable |
| `apc-netshelter` | search `apc-apdu` | RFC1213, SNMPv2, PowerNet | pduNamePlateTable, pduUnitStatusTable, pduInputPhaseStatusTable, pduExternalSensorTable, pduCircuitBreakerTable, pduOutletMeteredTable |
| `apc-netbotz` | search `apc-netbotz` | RFC1213, SNMPv2, PowerNet | wirelessSensorStatusTable |
| `hwgroup-ste2plus` | search `hwgroup-ste2plus` | RFC1213, SNMPv2, STE2-PLUS | sensTable (+ added sensValue) |
| `akcp-sensorprobe2pro` | search `akcp-sensor` | RFC1213, SNMPv2, SPAGENT | temperatureTable, humidityTable (+ added temperatureDegree, humidityPercent) |

`max_repetitions`: 100 (epa126), 50 (g5, netshelter); `retries: 1`, `timeout: 5s` on all
(mirrors operator defaults).

## Test-host mapping (test-inventory.yaml, all v2c, 127.0.0.1)

| Host | device_type | module | auth (community) |
|------|-------------|--------|------------------|
| DC-ROOM-ROW-01 | panduit-epa126 | panduit-epa126 | panduit-gen3 |
| DC-ROOM-ROW-02 | panduit-epa126 | panduit-epa126 | panduit-gen3-gen6pdu |
| DC-ROOM-ROW-03 | panduit-p48fe04 | panduit-g5 | panduit-gen5 |
| DC-ROOM-ROW-04 | panduit-p36f50g-32a3wye | panduit-g5 | panduit-gen6 |
| DC-ROOM-ROW-05 | hwgroup-ste2plus | hwgroup-ste2plus | hwgroup-ste2plus |
| DC-ROOM-ROW-06 | apc-netbotz | apc-netbotz | apc-netbotz |
| DC-ROOM-ROW-07 | akcp-sensorprobe2pro | akcp-sensorprobe2pro | akcp-sensorprobe2pro |

## Validation (tester) — PASS

| Check | Result |
|-------|--------|
| YAML parses (generator.yml, snmp-scrape.yml) | PASS |
| generator.yml uses only valid `ModuleConfig` keys (no `get`/`use_mib`) | PASS |
| Field names match snmp_exporter Go structs | PASS |
| Override `type` values valid (DisplayString/EnumAsInfo/gauge) | PASS |
| Auth struct fields valid, all v2c + community | PASS |
| All 6 families present as modules | PASS |
| All operator tables + common scalars covered | PASS (0 missing) |
| `max_repetitions` matches operator (100/50/50) | PASS |
| `retries: 1`, `timeout: 5s` on all modules | PASS |
| 7 test hosts -> correct module + auth | PASS |
| Communities match test-inventory.yaml | PASS (0 missing/extra) |

## How to build & run

```sh
# From the snmp_exporter generator/ dir:
make generator
mkdir -p /tmp/dcm-mibs   # place the 7 vendor MIBs here
MIBDIRS=/tmp/dcm-mibs ./generator generate \
  -m /tmp/dcm-mibs \
  -g $HOME/src/opencode/dc-operator-rewrite/prometheus-snmp/generator/generator.yml \
  -o $HOME/src/opencode/dc-operator-rewrite/prometheus-snmp/snmp.yml

./snmp_exporter --config.file=snmp.yml       # listens on :9116
curl 'http://127.0.0.1:9116/snmp?target=127.0.0.1&module=panduit-epa126&auth=panduit-gen3'
```

## Honest gaps / caveats

1. **Cannot run `generator generate` here** — it is a cgo/NetSNMP binary and the **7 required
   MIBs (RFC1213-MIB, SNMPv2-MIB, HAWK-I2-MIB, PANDUIT-MIB, PowerNet-MIB, STE2-PLUS-MIB,
   SPAGENT-MIB) are in neither repo**; they must be sourced from the vendors / Telegraf image.
   Validation was therefore static/schema-level + coverage cross-check.
2. **`lookups.source_indexes` names are best-effort guesses**; the generator derives the real
   index labels from each MIB's INDEX clause. Verify/adjust against the generated `snmp.yml`
   after the first `generate` (the single most likely field to need a fix).
3. **Added sensor value columns** (`sensValue`, `temperatureDegree`, `humidityPercent`) are
   inferred — the operator used Starlark for these. Confirm names against the MIBs.
4. **`apc-netshelter`** (match `apc-apdu`) has no simulator test host; module is structurally
   complete but only exercised in production.
5. **SNMP traps (UDP/1162)** have no snmp_exporter equivalent and are out of scope.
6. **Targeting**: operator polled by hostname; rewrite prefers device IP (NetBox `oob_ip`).
7. **v3**: all test hosts are v2c; a commented `dcm_v3` auth template maps the operator's
   `config_context.dcm.observability.snmp.*` fields.
