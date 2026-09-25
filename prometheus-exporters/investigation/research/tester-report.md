# Tester Validation Report

Validation of the builder output (`generator/generator.yml`, `prometheus/snmp-scrape.yml`,
`module-selection.md`) against the researcher inputs (`research/operator-findings.md`,
`research/exporter-findings.md`) and the actual snmp_exporter source
(`/Users/I503834/src/github.com/snmp_exporter`).

## Summary: PASS (with documented, expected caveats)

| Check | Result |
|-------|--------|
| YAML parses (generator.yml, snmp-scrape.yml) | PASS |
| generator.yml uses only valid `ModuleConfig` keys | PASS |
| Field names match snmp_exporter Go structs | PASS |
| Override `type` values are valid metric types | PASS |
| Auth struct fields valid, all v2c + community | PASS |
| All 6 device families present as modules | PASS |
| All operator tables covered per family | PASS (0 missing) |
| Common scalars (sysUpTime/sysName/sysLocation) present | PASS |
| `max_repetitions` matches operator (100/50/50) | PASS |
| `retries: 1`, `timeout: 5s` on all modules (operator defaults) | PASS |
| 7 test hosts -> correct module + auth | PASS |
| Community auths match test-inventory.yaml | PASS (0 missing, 0 extra) |

## 1. Schema validity (cross-checked against Go structs)

Verified field names against the actual structs:
- `generator/config.go` `ModuleConfig`: `walk, lookups, ,inline, overrides, filters` — builder
  uses only `walk/lookups/overrides` + inlined `max_repetitions/retries/timeout`. OK.
- `generator/config.go` `Lookup`: `source_indexes, lookup, drop_source_indexes, display_hint` —
  builder uses `source_indexes/lookup/drop_source_indexes`. OK.
- `generator/config.go` `MetricOverrides`: includes `type, name, scale, ignore, help, ...` —
  builder uses `name/type` (and could use scale/ignore). OK.
- `config/config.go` `Auth`: `community, version, username, security_level, password,
  auth_protocol, priv_protocol, priv_password, context_name` — builder v3 template uses exactly
  these. OK.
- `config/config.go` `WalkParams`: `max_repetitions, retries, timeout, ...` — inlined at module
  level as required. OK.

No forbidden fields (`get`, `use_mib`) present — confirmed the generator would not reject the
file for unknown keys under strict YAML.

Override `type` values used: `DisplayString`, `EnumAsInfo`, `gauge` — all present in
`generator/tree.go:174,184` metricType handling. OK.

## 2. OID coverage vs. operator-findings.md

Every table and the common scalars from the research doc appear in each module's `walk`:

- panduit-epa126: pduTable, pduMonTable, pduMon3PhTable, ipTHATable, ipContTable, idmTable,
  pduP2BrCctMonitorTable, pduOutTable — all present.
- panduit-g5: pdug5IdentTable, pdug5InputTable, pdug5InputPhaseTable, pdug5TemperatureTable,
  pdug5HumidityTable, pdug5GroupTable, pdug5OutletTable — all present.
- apc-netshelter: pduNamePlateTable, pduUnitStatusTable, pduInputPhaseStatusTable,
  pduExternalSensorTable, pduCircuitBreakerTable, pduOutletMeteredTable — all present.
- apc-netbotz: wirelessSensorStatusTable — present.
- hwgroup-ste2plus: sensTable (+added sensValue) — present.
- akcp-sensorprobe2pro: temperatureTable, humidityTable (+added temperatureDegree,
  humidityPercent) — present.

## 3. Mapping fidelity

- Module selection (`module-selection.md`) matches operator match rules in
  `roles/agent/defaults/main.yml:36-60`: exact `panduit-epa126`; search `panduit-p`,
  `apc-apdu`, `hwgroup-ste2plus`, `apc-netbotz`, `akcp-sensor`.
- All 7 test hosts map to the correct module + community-auth (verified programmatically).
- Auth v3 field mapping matches the operator's `config_context.dcm.observability.snmp.*`
  JMESPath (`defaults/main.yml:16-25`).

## 4. Findings / recommendations (non-blocking)

1. **Index label names in `lookups.source_indexes` are unverified guesses**
   (`pduIndex`, `wirelessSensorStatusIndex`, `temperatureIndex`, `humidityIndex`,
   `pdug5IdentIndex`, etc.). The generator derives real index labels from each table's MIB
   INDEX clause. After the first `generator generate`, inspect the produced `snmp.yml`
   `indexes:` blocks and correct any `source_indexes` that don't match. This is the single most
   likely field to need adjustment. **Cannot be verified here** because the vendor MIBs are not
   available (confirmed: none in either repo).

2. **Added value columns** (`sensValue`, `temperatureDegree`, `humidityPercent`) are inferred
   object names; the operator did not poll them (it used Starlark). Confirm exact names against
   STE2-PLUS-MIB / SPAGENT-MIB. If wrong, generation will emit a "cannot find" and (with
   `--fail-on-parse-errors`, default) exit non-zero — a clear signal to fix.

3. **`__param_auth` self-relabel is redundant** in `snmp-scrape.yml`: Prometheus already turns a
   target label named `__param_auth` into the `auth` URL query param, so the
   `source_labels:[__param_auth] -> target_label:__param_auth` relabel step is a harmless no-op.
   Can be removed for clarity. Functionally correct as-is.

4. **`apc-netshelter` untested by inventory** — no simulator host (match `apc-apdu`). Module is
   structurally complete but only exercised in production.

5. **Cannot run `generator generate`** end-to-end here: the generator is a cgo binary requiring
   NetSNMP + the 7 vendor MIBs, none of which are present. Validation is therefore
   static/schema-level plus coverage cross-check, which is the maximum possible without MIBs.

## 5. Verdict

The builder output is a faithful, schema-valid snmp_exporter rewrite of the operator's SNMP
polling. It will require one iteration of `generator generate` with the real vendor MIBs to
confirm/adjust the `source_indexes` label names and the added sensor-value object names — this
is expected and documented in `README.md` gaps and above.
