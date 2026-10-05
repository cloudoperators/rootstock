# Manual to test Prometheus collector locally

To test the Prometheus collector locally you can run the make command
`make prometheus-local-run`, which will:

1. Build the Go binary
2. Spin up the Redfish mockup server
3. Spin up local Prometheus collector
4. Run the Go binary with test Redfish credentials

The Prometheus collector uses the `prometheus.yaml` config file, which
configures probing the rootstock service every 30 seconds with 2 targets (PDU
IDs "1" and "2"). The mock Redfish server returns values stored in the
`testdata/pdu-mockup/redfish/v1` directory.

## Available Commands

- `make prometheus-local-run` - Full setup and run the server
- `make prometheus-local-setup` - Only setup (build, start mockup, start
  Prometheus)
- `make prometheus-local-cleanup` - Teardown (stop and remove Docker containers)
- `make run-redfish` - Run the rootstock binary with default credentials

## Accessing Services

- **Rootstock server**: http://localhost:9220
  - Probe endpoint:
    http://localhost:9220/probe?target=http://localhost:8000&labels=1
- **Prometheus UI**: http://localhost:9090
- **Redfish mockup**: http://localhost:8000

After finishing testing, run `make prometheus-local-cleanup` to remove Docker
containers related to the test.
