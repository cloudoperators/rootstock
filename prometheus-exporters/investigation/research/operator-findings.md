# DC Observability Operator — SNMP Research (for snmp_exporter rewrite)

Research target: `/Users/I503834/src/github.tools.sap/dc-observability-operator/` (`tmp/` ignored entirely).
Produced by the **researcher** role.

**Verification note vs. prior `../../research-findings.md`:** The prior doc was broadly correct on
structure but had several inaccuracies corrected below (test-inventory device_types/communities,
omitted table columns). All numeric OIDs the prior doc discussed exist only in the ignored
`tmp/snmpcollector/oids.go`; the operator source itself has zero numeric OIDs.

---

## 1. Device families and match rules

Source: `roles/agent/defaults/main.yml:36-60`. Six families. `match.op` is a Jinja test applied to
`device_type` (`device_types[0]`); `match.value` is its argument. Evaluated in listed order
(`input-snmp.toml.j2:2-5`). The `name` is also written as the `processor` tag.

| # | family name | match op | match value | input template |
|---|-------------|----------|-------------|----------------|
| 1 | `panduit-epa126` | `equalto` | `panduit-epa126` | `devices/panduit-epa126/input.toml.j2` |
| 2 | `panduit-g5` | `search` | `panduit-p` | `devices/panduit-g5/input.toml.j2` |
| 3 | `apc-netshelter` | `search` | `apc-apdu` | `devices/apc-netshelter/input.toml.j2` |
| 4 | `hwgroup-ste2plus` | `search` | `hwgroup-ste2plus` | `devices/hwgroup-ste2plus/input.toml.j2` |
| 5 | `apc-netbotz` | `search` | `apc-netbotz` | `devices/apc-netbotz/input.toml.j2` |
| 6 | `akcp-sensorprobe2pro` | `search` | `akcp-sensor` | `devices/akcp-sensorprobe2pro/input.toml.j2` |

`search` = Python `re.search` substring/regex; `equalto` = exact. `panduit-epa126` matched first by
exact match so EPA126 gateways don't fall through to `panduit-p`.

---

## 2. Complete SNMP object inventory per family

All OIDs are **symbolic** (`MIB::object`). `(tag)` = SNMP tag/label; `(metric)` = numeric metric.
Scalars end in `.0`/`.1` (GET); tables are walked.

### 2.1 `panduit-epa126` — `devices/panduit-epa126/input.toml.j2` (5 blocks)

**Block 1 — Basic PDU info (`:2-100`)** — Scalars:
- `RFC1213-MIB::sysUpTime.0` -> `system_uptime` (metric, int)
- `SNMPv2-MIB::sysName.0` -> `host`, `hw_name`, `hw_parent` (tags)
- `SNMPv2-MIB::sysLocation.0` -> `hw_id` (tag)
- `HAWK-I2-MIB::platModelName.0` -> `model` (tag)
- `HAWK-I2-MIB::invSerialNum.0` -> `serial_number` (tag)
- `HAWK-I2-MIB::platFwRev.0` -> `firmware_revision` (tag)

Table `HAWK-I2-MIB::pduTable` (index_as_tag=false; inherit hw_parent,host) `:37-76`:
pduName->hw_name, pduMode->pduMode, pduFwVersCPU->pdu_firmware_revision,
pduFwVersMeter->pdu_meter_firmware_revision, pduCommsFail->hw_status, pduType->model,
pduNumber->hw_id (all tags), pduMeanKVAUCL->pduMeanKVAUCL (metric).

Table `HAWK-I2-MIB::pduMonTable` (inherit host) `:78-89`: pduMonPduNumber->hw_id, pduCircuitName->hw_name (tags).

Table `HAWK-I2-MIB::pduMon3PhTable` (inherit host) `:91-98`: pdu3PhPduNumber->hw_id (tag).

**Block 2 — Env sensors (`:102-164`)** — Scalar sysName.0->host (tag).
- Table `HAWK-I2-MIB::ipTHATable` (inherit hw_parent,host) `:109-129`: ipTHAChan->hw_id,
  ipTHAName->hw_name, ipTHAName->sensor_location, ipTHAType->hw_type (tags).
- Table `HAWK-I2-MIB::ipContTable` (inherit hw_parent,host) `:131-146`: ipContChan->hw_id,
  ipContName->hw_name, ipContName->sensor_location (tags).
- Table `HAWK-I2-MIB::idmTable` (inherit hw_parent,host) `:148-162`: idmNumber->hw_id,
  idmNumber->hw_name, idmVersion->idm_firmware_revision (tags).

**Block 3 — Breakers (`:166-199`), max_repetitions=50** — Scalar sysName.0->host.
Table `HAWK-I2-MIB::pduP2BrCctMonitorTable` (inherit host) `:174-197`:
pduP2BrCktMonBrCctNumber->hw_id, pduP2BrCktMonBrCctID->hw_name, pduP2BrCktMonBrCctPhases->hw_phase_name,
pduP2BrCktMonPduNumber->(default), pduP2BrCktMonBrCctBreakerOutletMap->(default) (all tags).

**Block 4 — Outlets (batched loop), max_repetitions=100** — batch_size=(interval>=5?250:10).
Scalar sysName.0->host. Table `HAWK-I2-MIB::pduOutTable` (inherit host) `:212-228`:
pduOutNumber->hw_id, pduOutName->hw_name, pduOutOn->hw_status (tags).

### 2.2 `panduit-g5` — `devices/panduit-g5/input.toml.j2` (4 blocks; MIB `PANDUIT-MIB`)

Scalars: sysUpTime.0->system_uptime(metric), sysName.0->host/hw_name(tags), sysLocation.0->hw_id(tag),
`PANDUIT-MIB::pdug5Manufacturer.1`->vendor, `pdug5PartNumber.1`->model, `pdug5SerialNumber.1`->serial_number,
`pdug5FirmwareVersion.1`->firmware_revision (tags).

Tables:
- `pdug5IdentTable` (inherit host): pdug5Name->hw_name, pdug5FirmwareVersion->pdu_firmware_revision,
  pdug5Status->hw_status, pdug5Manufacturer->vendor, pdug5PartNumber->model, pdug5IdentIndex->hw_id,
  pdug5Model->(default) (tags).
- `pdug5InputTable` (index_as_tag=true; inherit host): pdug5InputType->phase_mode (tag).
- `pdug5InputPhaseTable` (index_as_tag=true; inherit host): pdug5InputPhaseIndex->hw_id, ->hw_name (tags).
- `pdug5TemperatureTable` (inherit host): pdug5TemperatureIndex->hw_id, pdug5TemperatureName->hw_name,
  pdug5TemperatureProbeStatus->hw_status (tags).
- `pdug5HumidityTable` (inherit host): pdug5HumidityIndex->hw_id, pdug5HumidityName->hw_name,
  pdug5HumidityProbeStatus->hw_status (tags).
- `pdug5GroupTable` (index_as_tag=true; inherit host), max_repetitions=50: pdug5GroupName->hw_name,
  pdug5GroupVoltageMeasType->hw_phase_name, pdug5GroupBreakerStatus->hw_status (tags).
- `pdug5OutletTable` (index_as_tag=true; inherit host), max_repetitions=50, batched(interval>=5?30:10):
  pdug5OutletIndex->hw_id, pdug5OutletName->hw_name, pdug5OutletBranch->breaker_id (tags).

### 2.3 `apc-netshelter` — `devices/apc-netshelter/input.toml.j2` (4 blocks; MIB `PowerNet-MIB`)

Scalars: sysUpTime.0->system_uptime(metric), sysName.0->host/hw_name/hw_parent, sysLocation.0->hw_id,
`pduNamePlatePartNumber.1`->model, `pduNamePlateSerialNumber.1`->serial_number,
`pduNamePlateFirmwareVersion.1`->firmware_revision (tags).

Tables:
- `pduNamePlateTable` (index_as_tag=true; inherit host): pduNamePlateName->hw_name,
  pduNamePlateFirmwareVersion->pdu_firmware_revision, pduNamePlateType->phase_mode,
  pduNamePlatePartNumber->model, pduNamePlateIndex->hw_id (tags),
  pduUnitPropertiesOutletCount->hw_pdu_outlets (metric), pduUnitPropertiesRatedPower->hw_power_rating (metric).
- `pduUnitStatusTable` (index_as_tag=true; inherit host): pduUnitStatusName->hw_name (tag).
- `pduInputPhaseStatusTable` (index_as_tag=true; inherit host): pduInputPhaseStatusIndex->hw_id, ->hw_name (tags).
- `pduExternalSensorTable` (inherit host): pduExternalSensorStatusValue->(metric),
  pduExternalSensorNamePlateIdentifier->hw_id, pduExternalSensorNamePlateType->hw_type,
  pduExternalSensorNamePlateName->hw_name, pduExternalSensorNamePlateLocation->sensor_location,
  pduExternalSensorNamePlateProductCode->model, pduExternalSensorStatusCommStatus->hw_status.
- `pduCircuitBreakerTable` (index_as_tag=true; inherit host), max_repetitions=50:
  pduCircuitBreakerConfigIndex->hw_id, pduCircuitBreakerName->hw_name,
  pduCircuitBreakerStatusCurrent->hw_current(metric), pduCircuitBreakerPropertiesCurrentRating->hw_current_rating(metric).
- `pduOutletMeteredTable` (index_as_tag=true; inherit host), max_repetitions=50, batched(interval>=5?30:10):
  pduOutletMeteredPropertiesNumber->hw_id, ...Name->hw_name, ...CircuitBreaker->breaker_id (tags),
  ...CurrentRating->hw_current_rating, ...StatusCurrent->hw_current, ...StatusPowerFactor->(metric),
  ...StatusActivePower->hw_power, ...PropertiesPowerRating->hw_power_rating, ...StatusResettableEnergy->hw_energy (metrics).

### 2.4 `hwgroup-ste2plus` — (MIB `STE2-PLUS-MIB`)

Scalars: sysUpTime.0->system_uptime(metric), sysName.0->host/hw_name/hw_parent, sysLocation.0->hw_id,
sysDescr.0->model (tags).
Table `STE2-PLUS-MIB::sensTable` (inherit hw_parent,host): sensID->hw_id, sensName->hw_name,
sensState->hw_status, sensUnit->hw_type (all tags). NOTE: no numeric value column polled — value likely
derived by Starlark. For snmp_exporter you likely want to ADD `sensValue`.

### 2.5 `apc-netbotz` — (MIB `PowerNet-MIB`)

Scalars: sysUpTime.0->system_uptime(metric), sysName.0->host/hw_name/hw_parent, sysLocation.0->hw_id,
`memModulesStatusModelNumber.0`->model, `...SerialNumber.0`->serial_number, `...FirmwareRev.0`->firmware_revision (tags).
Table `PowerNet-MIB::wirelessSensorStatusTable` (inherit hw_parent,host): wirelessSensorStatusExtAddr->hw_id,
...Name->hw_name, ...Name->hw_sensor_location, ...Location->location, ...CommStatus->hw_status (tags).

### 2.6 `akcp-sensorprobe2pro` — (MIB `SPAGENT-MIB`)

Scalars: sysUpTime.0->system_uptime(metric), sysName.0->host/hw_name/hw_parent, sysLocation.0->hw_id,
`cfgSystemDescription.0`->model (tags).
Tables:
- `SPAGENT-MIB::temperatureTable` (inherit hw_parent,host): temperatureId->hw_id, temperatureDescription->hw_name,
  temperatureType->hw_type, temperatureStatus->hw_status (tags). NOTE: add `temperatureDegree` for value.
- `SPAGENT-MIB::humidityTable` (inherit hw_parent,host): humidityId->hw_id, humidityDescription->hw_name,
  humidityStatus->hw_status (tags). NOTE: add `humidityPercent` for value.

---

## 3. MIBs referenced

`RFC1213-MIB`, `SNMPv2-MIB` (all families); `HAWK-I2-MIB` (epa126); `PANDUIT-MIB` (g5);
`PowerNet-MIB` (netshelter, netbotz); `STE2-PLUS-MIB` (hwgroup); `SPAGENT-MIB` (akcp). 7 distinct MIBs.
No `.mib` files in the repo; they live in the Telegraf image.

## 4. Per-input SNMP settings

Shared block `input-snmp-agent-configuration.toml.j2`: agents=list of **hostnames**, interval="{n}m",
timeout="{n}s", version=2|3, community="...", retries=n, agent_host_tag="source". **No port** set ->
Telegraf default UDP/161.
max_repetitions: epa126 breakers=50/outlets=100; g5 breakers=50/outlets=50; netshelter breakers=50/outlets=50;
basic+sensor blocks and hwgroup/netbotz/akcp use default (50).
Defaults: agent_interval "10" min, agent_timeout 5 s.

## 5. Auth model

v2c: version=2, community always emitted. v3 block only when `agent_snmp_version == '3'` (string compare —
latent bug, may never render): sec_name, auth_protocol, auth_password, sec_level, context_name,
priv_protocol, priv_password.

Defaults (`roles/agent/defaults/main.yml:70-80`): version=2, community="public", retries=1,
username='user', auth_protocol='', auth_password='', sec_level='authNoPriv', context_name='',
priv_protocol='', priv_password=''.

NetBox `config_context[0].dcm.observability.snmp.*` -> vars (JMESPath `:7-25`): version, community, retries,
username->sec_name, authProtocol->auth_protocol, authPassword->auth_password, security->sec_level,
contextName->context_name, privProtocol->priv_protocol, privPassword->priv_password. Also ansible_host(IP),
inventory_hostname, device_type, status.value, pollingIntervalMinutes->interval, timeoutSeconds->timeout.

Grouping: `configuration_group_key = device_type + '-' + join(agent_* values)` batches identical hosts.

## 6. Inventory

NetBox `nb_inventory` (`config/manager/ansible-inventory.yaml`): config_context=true, oob_ip_as_primary_ip=true,
group_by=[device_types], query_filters role in {pdu-gateway,pdu-red,pdu-yellow},
device_query_filters {has_oob_ip:true, tag__n:dcm-observability-disabled, status:active}, cache 3600s.
Reconcile hourly (`watches.yaml`).

Test inventory (`test-inventory.yaml`, 7 loopback hosts, all v2c, all 127.0.0.1):

| host | device_type | family | community |
|------|-------------|--------|-----------|
| DC-ROOM-ROW-01 | panduit-epa126 | panduit-epa126 | panduit-gen3 |
| DC-ROOM-ROW-02 | panduit-epa126 | panduit-epa126 | panduit-gen3-gen6pdu |
| DC-ROOM-ROW-03 | panduit-p48fe04 | panduit-g5 | panduit-gen5 |
| DC-ROOM-ROW-04 | panduit-p36f50g-32a3wye | panduit-g5 | panduit-gen6 |
| DC-ROOM-ROW-05 | hwgroup-ste2plus | hwgroup-ste2plus | hwgroup-ste2plus |
| DC-ROOM-ROW-06 | apc-netbotz | apc-netbotz | apc-netbotz |
| DC-ROOM-ROW-07 | akcp-sensorprobe2pro | akcp-sensorprobe2pro | akcp-sensorprobe2pro |

No test host exercises `apc-netshelter` (match `apc-apdu`).

## 7. Ports / traps

Poll 161 (implicit). Trap collector `input-snmp-trap-collector.toml.j2`: `[[inputs.snmp_trap]]`
service_address="udp://:1162", version="2c". K8s Service `telegraf-snmp-service` port 162 -> targetPort 1162 UDP.
snmp_exporter has NO trap equivalent — traps are out of scope for the rewrite (needs a separate trap sink).

## 8. Numeric OIDs

None in operator source. `1.3.6.1` matches only in ignored `tmp/`. OIDs are symbolic only; the generator
must resolve them from the 7 MIBs at generate time.
