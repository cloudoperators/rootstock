---
layout: home

hero:
  name: Rootstock
  text: Data Center Telemetry Collection
  image:
    src: /assets/logo/rootstock_logo.svg
    alt: rootstock logo
  tagline: >
    The hidden foundation that carries energy and carbon telemetry out of
    your data center hardware and into Greenhouse, ready for dashboards and
    carbon accounting.
  actions:
    - theme: brand
      text: Get Started
      link: /getting-started
    - theme: alt
      text: View on GitHub
      link: https://github.com/cloudoperators/rootstock

features:
  - icon: 🚀
    title: Getting Started
    details: >
      Deploy rootstock to your cluster with Helm. Connects to PDUs via
      Redfish and exposes metrics for Prometheus to scrape.
    link: /getting-started
    linkText: Deploy rootstock

  - icon: 🏗️
    title: Architecture
    details: >
      Three-component pipeline: HTTP Server, OTEL Collector, and Redfish
      Client. Multi-target pattern supports 100+ racks from a single pod.
    link: /concepts/architecture
    linkText: Architecture overview

  - icon: 📋
    title: Design
    details: >
      Goals, requirements, security model, and multi-vendor strategy.
      Starts with Panduit via Redfish; extensible to other vendors.
    link: /concepts/design
    linkText: Read the design
---
