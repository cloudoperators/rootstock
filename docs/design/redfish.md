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

This document describes the Redfish client implementation for Rootstock, which is used to communicate with PDU (Power Distribution Unit) devices to collect energy and sustainability metrics.

## Implementation Approach

There are two possible ways to implement the Redfish client:

1. **Use Go library (gofish)**: Leverage the existing `gofish` library, with raw Redfish API calls for endpoints not implemented in the library
2. **Implement raw Redfish only**: Direct HTTP/HTTPS calls to Redfish API endpoints

### Decision: Raw Redfish Implementation

We decided to implement **raw Redfish** for the following reasons:
- Uncertainty about gofish compatibility with Panduit PDUs (our initial target device)
- Full control over API calls and response handling
- Simpler dependency management
- Flexibility to handle vendor-specific Redfish implementations
- Easier to extend for multiple PDU vendors 

## Client Structure

The Redfish client is implemented as a Go struct with the following fields:

```go
type Client struct {
    baseURL    *url.URL      // Base URL of the PDU Redfish API (e.g., https://pdu01.example.com)
    httpClient *http.Client  // HTTP client with timeout and TLS configuration

    username   string        // Authentication username
    password   string        // Authentication password

    authToken  string        // Session authentication token (X-Auth-Token)
    sessionURL string        // Session URL for logout

    logger     *slog.Logger  // Structured logger for debugging and audit
}
```

### Client Constructor

```go
func NewClient(baseURL, username, password string) (*Client, error) {
    parsedURL, err := url.Parse(baseURL)
    if err != nil {
        return nil, fmt.Errorf("invalid base URL: %w", err)
    }

    return &Client{
        baseURL:    parsedURL,
        username:   username,
        password:   password,
        httpClient: &http.Client{
            Timeout: 10 * time.Second,
            Transport: &http.Transport{
                TLSClientConfig: &tls.Config{
                    InsecureSkipVerify: false, // Set to true only for development
                },
            },
        },
        logger: slog.Default(),
    }, nil
}
```

## Authentication Flow

### Session-Based Authentication

The Redfish client uses session-based authentication for better security and performance:

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