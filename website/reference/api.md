# API Reference

## Rootstock HTTP endpoints

### GET /metrics

Scrapes a single PDU target and returns its metrics in Prometheus text format.

| Parameter | Type | Required | Description |
|---|---|---|---|
| `target` | string | yes | Hostname of the PDU to scrape (must be on the whitelist) |

**Request**

```bash
curl "http://localhost:8080/metrics?target=pdu1.example.com"
```

**Response** — `200 OK`, `Content-Type: text/plain; version=0.0.4`

```
# HELP pdu_power_watts Current power draw in watts
# TYPE pdu_power_watts gauge
pdu_power_watts{target="pdu1.example.com",outlet="1"} 342.5
pdu_power_watts{target="pdu1.example.com",outlet="2"} 289.1

# HELP pdu_energy_kwh Cumulative energy consumed in kilowatt-hours
# TYPE pdu_energy_kwh counter
pdu_energy_kwh{target="pdu1.example.com"} 12483.7

# HELP pdu_power_factor Power factor (ratio of real to apparent power)
# TYPE pdu_power_factor gauge
pdu_power_factor{target="pdu1.example.com"} 0.97
```

**Error responses**

| Status | Cause | Resolution |
|---|---|---|
| `400 Bad Request` | Missing or empty `target` parameter | Include `?target=<host>` in the request |
| `403 Forbidden` | Target not on the hostname whitelist | Add the host to `config.targets.whitelist` |
| `502 Bad Gateway` | Redfish request to PDU failed | Check PDU reachability and credentials |
| `504 Gateway Timeout` | PDU did not respond within the configured timeout | Increase `config.timeout` or check PDU health |

---

### GET /health

Liveness probe. Returns `200 OK` when the process is running.

**Request**

```bash
curl http://localhost:8080/health
```

**Response** — `200 OK`, `Content-Type: application/json`

```json
{"status":"ok"}
```

---

### GET /ready

Readiness probe. Returns `200 OK` when the service is ready to handle scrape requests.

**Request**

```bash
curl http://localhost:8080/ready
```

**Response** — `200 OK`, `Content-Type: application/json`

```json
{"status":"ready"}
```

Returns `503 Service Unavailable` if the service is still initializing.

---

## Redfish endpoints queried on PDUs

Rootstock queries these endpoints on each target PDU. These are not rootstock endpoints — they are the Redfish API paths rootstock calls.

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/redfish/v1/` | Service root — confirms Redfish availability and version |
| `GET` | `/redfish/v1/PowerEquipment` | Power equipment index |
| `GET` | `/redfish/v1/PowerEquipment/RackPDUs` | Rack PDU collection; contains power, energy, and power factor readings |

All requests use HTTPS. Authentication is Basic Auth or Session Auth, configured via Kubernetes Secret.

## See also

- [Architecture](../concepts/architecture) — data flow and component descriptions
- [Getting Started](../getting-started) — deployment and Prometheus scrape configuration
