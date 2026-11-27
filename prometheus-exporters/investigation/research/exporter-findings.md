# snmp_exporter Configuration Authoring Guide

Repository: `/Users/I503834/src/github.com/snmp_exporter`. Produced by the **researcher-exporter** role.

Two YAML formats:
1. **`generator.yml`** — human-authored input to the `generator` tool (`generator/config.go:24`).
2. **`snmp.yml`** — machine-generated output consumed by the exporter at runtime
   (`config/config.go:118`). Auto-generated; must not be hand-edited (`README.md:143`).

---

## 1. Generator workflow (`generator generate`)

- cgo binary linked against NetSNMP; build locally (`generator/README.md:8-19`).
- Build: `make generator mibs` in `generator/`. Run: `make generate` ->
  `MIBDIRS=mibs ./generator --fail-on-parse-errors generate --output-path=../snmp.yml`
  (`generator/Makefile:119-120`).
- Manual: `./generator generate -m <mibdir> -g generator.yml -o snmp.yml` (`generator/README.md:41-47`).
- Flags: `--fail-on-parse-errors` (default true), `--snmp.mibopts` (default "eu"), `-m/--mibs-dir`
  (repeatable), `-g/--generator-path`, `-o/--output-path` (`generator/main.go:103-112`).
- `generate` loads MIBs (MIBS=ALL) via NetSNMP, strict-parses generator.yml, expands
  walks/lookups/overrides/filters into concrete metrics, re-parses output strictly, prepends warning
  header (`generator/main.go:35-101`).

Top-level `Config` (`generator/config.go:24-28`):
```yaml
auths:   { <name>: <Auth> }
modules: { <name>: <ModuleConfig> }
version: <int optional>
```

---

## 2. Generator MODULE schema (`ModuleConfig`, `generator/config.go:57-63`)

```
walk:      []string        # OIDs / MIB object names / table names / instances (MIB-qualify with ::)
lookups:   []Lookup
<WalkParams inline>        # max_repetitions, retries, timeout, use_unconnected_udp_socket, allow_nonincreasing_oids
overrides: map[metricName]MetricOverrides
filters:   {static:[], dynamic:[]}
```
- **No `get` field** and **no `use_mib` field** in generator module config. `get` only exists in the
  generated snmp.yml; the generator emits it (e.g. scalar `.0`, static filters).

**Lookup** (`generator/config.go:85-90`):
```
source_indexes: []string   # index label(s) identifying the row
lookup:         string     # object name/OID whose value augments/replaces the label
drop_source_indexes: bool  # remove source index labels afterwards
display_hint:   string     # RFC2579 DISPLAY-HINT; @mib uses the MIB's
```
Lookups can chain (order matters). All source_indexes must be present on the metric.

**MetricOverrides** (`generator/config.go:30-40`):
```
ignore: bool                 # drop metric (used for lookup-only objects)
regex_extracts: map[suffix][]{regex,value}   # ''-key replaces value in place
datetime_pattern: string     # with type: ParseDateAndTime
offset: float                # applied AFTER scale
scale:  float                # multiply sample (e.g. 0.1 for tenths)
type:   string               # override metric type (validated)
help:   string
name:   string               # rename metric
display_hint: string
```
Valid `type`: gauge, counter, OctetString, DateAndTime, ParseDateAndTime, ParseFloatString,
NTPTimeStamp, DisplayString, PhysAddress48, Float, Double, InetAddressIPv4/IPv6/InetAddress/
InetAddressMissingSize, EnumAsInfo, EnumAsStateSet, Bits (`generator/tree.go:159-196`).
Enum used as index/lookup defaults to `EnumAsInfo` (override with `type: gauge` for numeric).
Alertable states -> `EnumAsStateSet`.

**filters** (`config/config.go:223-236`): static (resolved at gen time -> gets; indices must be ints)
and dynamic (passed to runtime; exporter walks `oid`, keeps rows whose value is in `values`).

---

## 3. AUTHS schema (`config/config.go:29-39`)

```
community:      Secret   # v1/v2, default "public"
version:        int      # 1|2|3, default 2
username:       string   # v3
security_level: string   # noAuthNoPriv(default)|authNoPriv|authPriv
password:       Secret    # authKey; required authNoPriv/authPriv
auth_protocol:  string   # MD5(default)|SHA|SHA224|SHA256|SHA384|SHA512
priv_protocol:  string   # DES(default)|AES|AES192|AES192C|AES256|AES256C
priv_password:  Secret    # privKey; required authPriv
context_name:   string
```
Defaults `DefaultAuth` (`config/config.go:95-101`). Modules do NOT bind an auth; auth chosen at query
time via `auth` URL param. Env-var expansion (`${VAR}`) supported for username/password/priv_password
with `--config.expand-environment-variables`.

---

## 4. Generated snmp.yml schema (`config/config.go`)

- `Module` (`:132-139`): walk, get, metrics[], WalkParams inline, filters (dynamic only).
- `WalkParams` (`:124-130`, defaults `:102-108`): max_repetitions (default 25), retries (default 3),
  timeout (default 5s), use_unconnected_udp_socket, allow_nonincreasing_oids.
- `Metric` (`:238-251`): name, oid, type, help, indexes[], lookups[], regex_extracts, datetime_pattern,
  enum_values, offset, scale, display_hint.
- `Index` (`:271-277`): labelname, type, fixed_size, implied, enum_values.
- runtime `Lookup` (`:290-297`): labels[], labelname, oid, type, display_hint, enum_values
  (DIFFERENT from generator Lookup which uses source_indexes/lookup).

---

## 5. Runtime querying (`main.go`)

`GET /snmp?target=<ip>&module=<m>&auth=<a>`:
- `target` required; `[transport://]host[:port]`, default udp/161.
- `auth` optional, defaults `public_v2`.
- `module` optional, defaults `if_mib`; multi-module via comma or repeated param.
- `snmp_context`, `snmp_engineid`, `snmp_debug_packets` optional.
Listen `:9116`. `/config`, `/-/reload`, `/-/healthy`, `/metrics`.

**No built-in inventory** — Prometheus supplies targets via scrape_configs + relabeling
(`README.md:157-186`, `examples/sample-configs/prometheus-network-devices.yml`):
```yaml
scrape_configs:
  - job_name: snmp
    static_configs: [{targets: [192.168.1.2]}]
    metrics_path: /snmp
    params: {auth: [public_v2], module: [if_mib]}
    relabel_configs:
      - {source_labels: [__address__], target_label: __param_target}
      - {source_labels: [__param_target], target_label: instance}
      - {target_label: __address__, replacement: 127.0.0.1:9116}
```
`--config.file` repeatable + glob (ship snmp.yml + separate auth file). Strict unmarshalling.

---

## 6. MIBs & bundled snmp.yml

- `generator/mibs/` empty; `make mibs` downloads. Generator reads `MIBDIRS`/`-m`. Recommend one dir per
  device family. All IMPORT deps must be present or generation fails.
- Repo bundles a ~2.1 MB prebuilt `snmp.yml` (~65k lines) with only `public_v1`/`public_v2` auths.

**Existing PDU/power/environmental modules** (relevant but NONE match our vendors exactly):
apcups (PowerNet-MIB UPS + rPDU2* + probes), apcats, servertech_sentry3/4, raritan, raritan_pdu2,
raritan_assetmanagement, infrapower_pdu, liebert_pdu, cyberpower, powercom, ups_mib.

**NOT present (must author + supply MIBs):** Panduit, AKCP, HWgroup/STE2, NetBotz, Hawk (HAWK-I2-MIB).
=> All 6 of our families need NEW modules + their vendor MIBs.

---

## 7. Key gotchas for the builder

1. Author generator.yml, not snmp.yml.
2. Generator uses strict YAML — only allowed fields; no `get`, no `use_mib` in module config.
3. Generator Lookup (`source_indexes`+`lookup`) != runtime Lookup (`labels`+`labelname`+`oid`).
4. `walk` accepts MIB names/table names/numeric OIDs; MIB-qualify with `::`.
5. Tenths scaling -> `scale: 0.1`; string numbers -> `ParseFloatString` / regex_extracts.
6. Sensor state -> `EnumAsStateSet`; inventory enum labels -> `EnumAsInfo`.
7. Drop lookup-only objects with `ignore: true`.
8. Buggy PDUs: low `max_repetitions`, tune retries/timeout, `allow_nonincreasing_oids`.
9. Auths are query-time; define `public_v2` + v3 auths, select via `auth` param.
10. Supply MIBs for every vendor not already covered (all of ours).
