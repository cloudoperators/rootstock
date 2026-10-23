# Redfish Client Implementation

## Table of Contents

- [Overview](#overview)
- [Implementation Approach](#implementation-approach)
  - [Decision: Raw Redfish Implementation](#decision-raw-redfish-implementation)
- [Client Structure](#client-structure)
  - [Client Constructor](#client-constructor)
- [Authentication Flow](#authentication-flow)
  - [Session-Based Authentication](#session-based-authentication)

---

## Overview

This document describes the Redfish client implementation for Rootstock, which
is used to communicate with PDU (Power Distribution Unit) devices to collect
energy and sustainability metrics.

## Implementation Approach

There are two possible ways to implement the Redfish client:

1. **Use Go library (gofish)**: Leverage the existing `gofish` library, with raw
   Redfish API calls for endpoints not implemented in the library
2. **Implement raw Redfish only**: Direct HTTP/HTTPS calls to Redfish API
   endpoints

### Decision: Raw Redfish Implementation

We decided to implement **raw Redfish** for the following reasons:

- Uncertainty about gofish compatibility with Panduit PDUs (our initial target
  device)
- Full control over API calls and response handling
- Simpler dependency management
- Flexibility to handle vendor-specific Redfish implementations
- Easier to extend for multiple PDU vendors

## Client Structure

The Redfish client is implemented using an interface-based design with a
concrete implementation called `ClientImpl`. The implementation maintains the
following fields:

**ClientImpl Structure:**

- **baseURL**: A parsed URL object containing the base address of the PDU
  Redfish API (for example, https://pdu01.example.com)
- **httpClient**: An HTTP client configured with timeout settings and TLS
  security options
- **username**: The authentication username for connecting to the PDU
- **password**: The authentication password for connecting to the PDU
- **authToken**: The session authentication token (X-Auth-Token) received after
  successful login, used for subsequent API requests
- **sessionURL**: The session URL path needed to properly logout and clean up
  the session
- **logger**: A structured logger (MinInterface) for debugging and audit trail
  purposes
- **timeout**: A configurable timeout duration for HTTP requests

### Client Constructor

The `NewClient` function creates and initializes a new Redfish client instance.
It takes five parameters: a parsed base URL, a logger interface, username,
password, and timeout duration.

**Initialization steps:**

1. **Logger Validation**: If no logger is provided (nil), a default logger is
   assigned to ensure logging is always available.
2. **Client Creation**: A new ClientImpl instance is created with:
   - The provided parsed base URL
   - The provided or default logger
   - The username and password credentials
   - The specified timeout duration
   - An HTTP client configured with:
     - The specified timeout for all requests
     - TLS (Transport Layer Security) configuration with certificate
       verification enabled by default (InsecureSkipVerify is set to false)
   - Empty authToken and sessionURL (these are populated after successful login)

The function returns the initialized client ready for use.

## Authentication Flow

### Session-Based Authentication

The Redfish client uses session-based authentication for better security and
performance:

```
┌─────────────────────────────────────────────────────────────────┐
│                    Redfish Client Lifecycle                      │
└─────────────────────────────────────────────────────────────────┘

1. NewClient(baseURL, username, password)
     │
     │  • Parse and validate base URL
     │  • Initialize HTTP client with timeout and TLS config
     │
     │
     ▼
2. Login()
     │
     ├── POST /redfish/v1/SessionService/Sessions
     │   Body: {"UserName": "...", "Password": "..."}
     │
     ├── Response Headers:
     │   • X-Auth-Token: <session-token>
     │   • Location: /redfish/v1/SessionService/Sessions/{id}
     │
     ├── Store authToken for subsequent requests
     │
     └── Store sessionURL for logout
     │
     ▼
3. Get(endpoint)
     │
     ├── GET {baseURL}{endpoint}
     │   Headers:
     │   • X-Auth-Token: <session-token>
     │   • Content-Type: application/json
     │
     └── Return parsed JSON response
     │
     ▼
4. (Repeated API Calls)
     │
     ├── GetPowerEquipment()
     ├── GetRackPDUs()
     ├── GetPDUMetrics(pduID)
     │
     └── All use X-Auth-Token
     │
     ▼
5. Logout() / Close()
     │
     └── DELETE {sessionURL}
         • Clean up session on PDU
         • Invalidate token
```
