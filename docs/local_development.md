# Local development

This document describes how to get the local rootstock setup to work.

## Cobra CLI

Rootstock is a [cobra cli](https://github.com/spf13/cobra). That means that its
API with commands and flags can be explored using the cli-parameter `-h` or
`--help` like

```shell
rootstock -h

Available Commands:
  completion  Generate the autocompletion script for the specified shell
  help        Help about any command
  run         run the rootstock server
  version     Returns the cli version

Flags:
      --add-trace              if this flag is set, error logs will have a stacktrace appended. (env: "ADD_TRACE")
      --config string          absolute config path (env: "CONFIG_PATH") (default "/config/.rootstock.yaml")
      --config-folder string   folder where the config files are stored (env: "CONFIG_FOLDER") (default "/config")
  -h, --help                   help for rootstock
  -f, --log-format string      if this flag is set, logs will be presented in text format - options: text, json (env: "LOG_FORMAT") (default "json")
  -l, --log-lvl string         application log level options: key or value of [debug info warn error] (env: "LOG_LEVEL") (default "info")
      --tmp-folder string      tmp folder used by rootstock (env: "TMP_FOLDER")

Use "rootstock [command] --help" for more information about a command.
```

## Configuration inputs

Rootstock accepts configuration input via 4 routes, ordered from least to
highest precedence:

- `defaults`
- `config-file`
- `environment variable`
- `commandline flag`

## 1-time config preparation

To start rootstock, prepare the following config files by running

```shell
mkdir -p tmp

echo "example_pw" > tmp/redfish_password

cat <<EOF >>tmp/.rootstock.yaml
rootstock:
  log:
    level: debug
    format: text
    # trace: true
  redfish:
    credentialsFile: tmp/redfish_password
  config:
    folder: .
EOF
```

## run rootstock locally

To run rootstock locally, make sure that the
[1-time setup](#1-time-config-preparation) is complete. Then execute

```shell
go run main.go run --config ./tmp/.rootstock.yaml
```

Alternatively, you can also forego a config file by directly running

```shell
go run main.go run \
  --redfish-credentials-file tmp/redfish_password \
  --config-folder . \
  --log-format text
```
