# Rootstock Target Architecture

## Package Overview

```txt
┌─────────────────────────────────────────────────────────────────┐
│                         External Consumers                      │
│                                                                 │
│   ┌─────────────┐    ┌──────────────────┐    ┌───────────────┐  │
│   │ Prometheus  │    │  OTLP Receiver   │    │  OTLP Receiver│  │
│   │  (scrape)   │    │  (pull/passive)  │    │  (push/active)│  │
│   └──────┬──────┘    └───────┬──────────┘    └───────┬───────┘  │
└──────────┼───────────────────┼───────────────────────┼──────────┘
           │                   │                       │
           ▼                   ▼                       ▼
┌─────────────────────────────────────────────────────────────────┐
│  exporter                                                       │
│                                                                 │
│   ┌─────────────┐    ┌──────────────────┐    ┌───────────────┐  │
│   │  prometheus │    │    otel-pull     │    │   otel-push   │  │
│   │  exporter   │    │    exporter      │    │   exporter    │  │
│   └──────┬──────┘    └───────┬──────────┘    └───────┬───────┘  │
└──────────┼───────────────────┼───────────────────────┼──────────┘
           │                   │                       │
           └───────────────────┴───────────────────────┘
                               │  read
                               ▼
┌─────────────────────────────────────────────────────────────────┐
│  store                                                          │
│                                                                 │
│   ┌─────────────────────────────────────────────────────────┐   │
│   │                  internal data store                    │   │
│   │              (keyed by source / target)                 │   │
│   └─────────────────────────────────────────────────────────┘   │
└──────────────────────────────┬──────────────────────────────────┘
                               ▲  write
                               │
┌──────────────────────────────┴──────────────────────────────────┐
│  collector                                                      │
│                                                                 │
│   ┌─────────────────────────────────────────────────────────┐   │
│   │  1. periodically call all sources                       │   │
│   │  2. send raw source data to processor                   │   │
│   │  3. write processor results to store                    │   │
│   └───────┬──────────────────────────────────────┬──────────┘   │
└───────────┼──────────────────────────────────────┼──────────────┘
            │  fetch raw data                      │  process
            ▼                                      ▼
┌───────────────────────────┐     ┌─────────────────────────────┐
│  source                   │     │  processor                  │
│                           │     │                             │
│   ┌───────────┐           │     │  computes internal model    │
│   │  redfish  │           │     │  data from a single         │
│   └───────────┘           │     │  upstream dataset           │
│   ┌───────────┐           │     │                             │
│   │   <xxx>   │           │     │   uses model package for    │
│   └───────────┘           │     │   translation               │
│                           │     └──────────────┬──────────────┘
│  establishes connections  │                    │
│  to upstream APIs         │                    │
└───────────┬───────────────┘                    │
            │  raw API response                  │  uses
            │                                    ▼
            │                     ┌──────────────────────────────┐
            │                     │  model                       │
            │                     │                              │
            │                     │  internal data model +       │
            │                     │  translation between         │
            │                     │  external APIs and           │
            │                     │  internal representation     │
            │                     └──────────────────────────────┘
            │
            ▼
┌─────────────────────────────────────────────────────────────────┐
│  Upstream APIs (Hardware)                                       │
│                                                                 │
│   ┌────────────┐   ┌────────────┐   ┌────────────┐              │
│   │   PDU 1    │   │   PDU 2    │   │   PDU N    │              │
│   │ Redfish API│   │ Redfish API│   │ Redfish API│              │
│   └────────────┘   └────────────┘   └────────────┘              │
└─────────────────────────────────────────────────────────────────┘
```

## Data Flow

```txt
source ──(raw data)──► processor ──(model.Result)──► collector ──(write)──► store
                           │                              │
                           ▼                              ▼
                         model                        exporter ──► consumer
                      (translation)                  (reads store)
```

## Package Responsibilities

| Package           | Responsibility                                                                              |
| ----------------- | ------------------------------------------------------------------------------------------- |
| `source/<type>`   | Connects to and fetches raw data from upstream APIs (e.g. Redfish)                          |
| `model`           | Internal data model; translation between external API responses and internal representation |
| `processor`       | Computes internal model results from a single upstream dataset                              |
| `store`           | In-process data store; holds latest processed results keyed by source/target                |
| `collector`       | Orchestration: drives periodic source polling, processor calls, and store writes            |
| `exporter/<type>` | Exposes store data to external consumers (Prometheus pull, OTLP pull, OTLP push)            |
