# Rootstock service roadmap

## service structure

- cli?
  - [cobra](https://github.com/spf13/cobra) +
    [viper](https://github.com/spf13/viper)
  - or simple with `envconfig`
- folder structure
  - `cmd/binName` for each binary's `main.go` file
  - pkg for packages that can be consumed by other projects
  - internal for remaining packages
  - top-level docs folder

## chart structure

- dynamic config loading?

## features

- list of features ordered from highest to lowest priority.
- priority is a non-linear combination of
  - usefulness
  - complexity
  -
