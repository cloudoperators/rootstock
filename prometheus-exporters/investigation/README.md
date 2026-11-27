# dc-observability-operator -> Prometheus snmp_exporter rewrite

This directory rewrites the SNMP polling performed by the
**dc-observability-operator** (`/Users/I503834/src/github.tools.sap/dc-observability-operator/`)
in terms of **instantiations of the Prometheus snmp_exporter**
(`/Users/I503834/src/github.com/snmp_exporter`).

The `tmp/` folder of the operator was ignored throughout.

## What the operator does (baseline)

The operator is an Operator-SDK **Ansible** operator. It does **not** poll SNMP itself:
it discovers PDUs / environmental sensors from **NetBox** (`nb_inventory`), renders
**Telegraf** TOML (`[[inputs.snmp]]` blocks per device family), and deploys Telegraf pods
that do the actual SNMP polling. It also runs a Telegraf SNMP **trap** receiver.

## What this rewrite produces

The snmp_exporter equivalent, where **Prometheus** scrapes the **snmp_exporter**, which
polls each device over SNMP:

| Operator concept | snmp_exporter equivalent |
|------------------|--------------------------|
| Telegraf `[[inputs.snmp]]` per device family | one snmp_exporter **module** per family |
| Per-family OID lists (symbolic `MIB::object`) | module `walk:` (generator resolves via MIBs) |
| Telegraf tag columns (`hw_id`, `hw_name`, ...) | module `lookups:` + `overrides` (`DisplayString`/`EnumAsInfo`) |
| `agent_snmp_community` / v3 params | snmp_exporter **auths** |
| NetBox `nb_inventory` dynamic discovery | Prometheus `http_sd`/`file_sd` (adapter) |
| `agent_interval` / `agent_timeout` / `retries` | `scrape_interval` / module `timeout` / `retries` |
| SNMP trap receiver (UDP/1162) | **no equivalent** — out of scope |

## Files

| File | Purpose |
|------|---------|
| `generator/generator.yml` | Human-authored generator input: `auths` + 6 `modules` (one per device family), with `walk`, `lookups`, `overrides`, `max_repetitions`, `retries`, `timeout`. |
| `prometheus/snmp-scrape.yml` | Prometheus `scrape_configs` reproducing the operator's polling for the 7 test hosts, plus the NetBox->service-discovery pattern for production. |
| `module-selection.md` | device_type match rule -> module, host -> module+auth, and auth field mapping. |
| `research/operator-findings.md` | Distilled operator SNMP facts (researcher). Authoritative source for OID coverage. |
| `research/exporter-findings.md` | Distilled snmp_exporter config/generator/runtime facts (researcher-exporter). |

## The 6 modules (family -> tables)

- **panduit-epa126** (HAWK-I2-MIB): sys scalars + `pduTable`, `pduMonTable`, `pduMon3PhTable`,
  `ipTHATable`, `ipContTable`, `idmTable`, `pduP2BrCctMonitorTable` (breakers), `pduOutTable`
  (outlets). `max_repetitions: 100`.
- **panduit-g5** (PANDUIT-MIB): sys scalars + `pdug5*` scalars + `pdug5IdentTable`,
  `pdug5InputTable`, `pdug5InputPhaseTable`, `pdug5TemperatureTable`, `pdug5HumidityTable`,
  `pdug5GroupTable` (breakers), `pdug5OutletTable` (outlets). `max_repetitions: 50`.
- **apc-netshelter** (PowerNet-MIB): sys scalars + `pduNamePlate*` + `pduNamePlateTable`,
  `pduUnitStatusTable`, `pduInputPhaseStatusTable`, `pduExternalSensorTable`,
  `pduCircuitBreakerTable`, `pduOutletMeteredTable`. `max_repetitions: 50`.
- **apc-netbotz** (PowerNet-MIB): sys scalars + `memModulesStatus*` + `wirelessSensorStatusTable`.
- **hwgroup-ste2plus** (STE2-PLUS-MIB): sys scalars + `sensTable` (+ added `sensValue`).
- **akcp-sensorprobe2pro** (SPAGENT-MIB): sys scalars + `temperatureTable`, `humidityTable`
  (+ added `temperatureDegree`, `humidityPercent`).

## How to build the runtime snmp.yml

The generator is a cgo binary linked against NetSNMP. From the snmp_exporter `generator/`
directory (see its README):

```sh
# 1. Build the generator and provide the vendor MIBs.
make generator

# 2. Put the 7 required MIBs into a mib dir (see "MIBs" below).
#    e.g. mkdir -p /tmp/dcm-mibs && cp <vendor mibs> /tmp/dcm-mibs/

# 3. Generate snmp.yml from this generator.yml.
MIBDIRS=/tmp/dcm-mibs ./generator generate \
  -m /tmp/dcm-mibs \
  -g /Users/I503834/src/opencode/dc-operator-rewrite/prometheus-snmp/generator/generator.yml \
  -o /Users/I503834/src/opencode/dc-operator-rewrite/prometheus-snmp/snmp.yml
```

## Running the exporter and querying it

```sh
./snmp_exporter --config.file=snmp.yml    # listens on :9116

# Manual query, mirroring what Prometheus does:
curl 'http://127.0.0.1:9116/snmp?target=127.0.0.1&module=panduit-epa126&auth=panduit-gen3'
```

Prometheus wiring is in `prometheus/snmp-scrape.yml`.

## MIBs (must be supplied — NOT in either repo)

The generator resolves symbolic OIDs to numeric OIDs from these 7 MIBs. None are present
in the operator repo or the snmp_exporter repo; source them from the vendors or the
Telegraf image used by the operator:

`RFC1213-MIB`, `SNMPv2-MIB`, `HAWK-I2-MIB`, `PANDUIT-MIB`, `PowerNet-MIB`,
`STE2-PLUS-MIB`, `SPAGENT-MIB`.

All IMPORT dependencies of these MIBs must also be present or generation fails.

## Honest gaps / caveats

1. **Numeric OIDs unknown until generation.** The operator uses only symbolic names; this
   `generator.yml` uses the same MIB-qualified names. Actual numeric OIDs are produced by
   `generator generate` once the MIBs are supplied.
2. **Index label names are best-effort.** The `lookups.source_indexes` entries
   (e.g. `pduIndex`, `wirelessSensorStatusIndex`, `temperatureIndex`) are inferred from the
   table/column naming. The generator derives the real index label names from the MIB INDEX
   clauses; verify and adjust `source_indexes` against the generated `snmp.yml` (they are the
   only fields that may need correction after a first `generate`).
3. **Added value columns.** For `hwgroup-ste2plus` and `akcp-sensorprobe2pro`, the operator
   template collected only tag columns and relied on Telegraf Starlark for the numeric value.
   We added `sensValue` / `temperatureDegree` / `humidityPercent` so the metric value is
   emitted directly. Confirm the exact object names against the vendor MIBs.
4. **apc-netshelter has no test host** (match `apc-apdu`); the module is untested by the
   simulator inventory.
5. **Traps out of scope.** The operator's UDP/1162 SNMP-trap receiver has no snmp_exporter
   equivalent and is not reproduced here.
6. **Targeting.** The operator polled by hostname (`inventory_hostname`, via `/etc/hosts`);
   this rewrite prefers the device IP (NetBox `oob_ip`) as the snmp_exporter `target`.
7. **v3.** All test hosts are v2c. The v3 auth is provided as a commented template in
   `generator.yml` mapping the operator's `config_context.dcm.observability.snmp.*` fields.
