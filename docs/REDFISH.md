# Redfish Client Implementation

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
     │  • Store credentials
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

### Alternative: Basic Authentication

For simpler scenarios or PDUs that don't support sessions:

```
Each Request:
     │
     ├── GET {baseURL}{endpoint}
     │   Headers:
     │   • Authorization: Basic <base64(username:password)>
     │   • Content-Type: application/json
     │
     └── Return response
```

## Key API Methods

### Login Method

```go
func (c *Client) Login() error {
    loginURL := c.baseURL.ResolveReference(&url.URL{Path: "/redfish/v1/SessionService/Sessions"})
    
    payload := map[string]string{
        "UserName": c.username,
        "Password": c.password,
    }
    
    body, err := json.Marshal(payload)
    if err != nil {
        return fmt.Errorf("failed to marshal login payload: %w", err)
    }
    
    req, err := http.NewRequest("POST", loginURL.String(), bytes.NewBuffer(body))
    if err != nil {
        return fmt.Errorf("failed to create login request: %w", err)
    }
    
    req.Header.Set("Content-Type", "application/json")
    
    resp, err := c.httpClient.Do(req)
    if err != nil {
        return fmt.Errorf("login request failed: %w", err)
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != http.StatusCreated {
        return fmt.Errorf("login failed with status: %d", resp.StatusCode)
    }
    
    // Store authentication token
    c.authToken = resp.Header.Get("X-Auth-Token")
    c.sessionURL = resp.Header.Get("Location")
    
    c.logger.Info("Successfully authenticated", "sessionURL", c.sessionURL)
    return nil
}
```

### Get Method

```go
func (c *Client) Get(endpoint string) ([]byte, error) {
    targetURL := c.baseURL.ResolveReference(&url.URL{Path: endpoint})
    
    req, err := http.NewRequest("GET", targetURL.String(), nil)
    if err != nil {
        return nil, fmt.Errorf("failed to create request: %w", err)
    }
    
    // Use session token if available, otherwise fall back to Basic Auth
    if c.authToken != "" {
        req.Header.Set("X-Auth-Token", c.authToken)
    } else {
        req.SetBasicAuth(c.username, c.password)
    }
    
    req.Header.Set("Content-Type", "application/json")
    
    resp, err := c.httpClient.Do(req)
    if err != nil {
        return nil, fmt.Errorf("request failed: %w", err)
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("request failed with status: %d", resp.StatusCode)
    }
    
    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return nil, fmt.Errorf("failed to read response body: %w", err)
    }
    
    return body, nil
}
```

### Logout Method

```go
func (c *Client) Logout() error {
    if c.sessionURL == "" {
        return nil // No session to close
    }
    
    req, err := http.NewRequest("DELETE", c.sessionURL, nil)
    if err != nil {
        return fmt.Errorf("failed to create logout request: %w", err)
    }
    
    req.Header.Set("X-Auth-Token", c.authToken)
    
    resp, err := c.httpClient.Do(req)
    if err != nil {
        return fmt.Errorf("logout request failed: %w", err)
    }
    defer resp.Body.Close()
    
    c.authToken = ""
    c.sessionURL = ""
    
    c.logger.Info("Successfully logged out")
    return nil
}

## PDU-Specific Methods

### Get Power Equipment

```go
func (c *Client) GetPowerEquipment() ([]byte, error) {
    return c.Get("/redfish/v1/PowerEquipment")
}
```

### Get Rack PDUs

```go
func (c *Client) GetRackPDUs() ([]byte, error) {
    return c.Get("/redfish/v1/PowerEquipment/RackPDUs")
}
```

### Get PDU Metrics

```go
func (c *Client) GetPDUMetrics(pduID string) ([]byte, error) {
    endpoint := fmt.Sprintf("/redfish/v1/PowerEquipment/RackPDUs/%s/Metrics", pduID)
    return c.Get(endpoint)
}
```

## Error Handling

The client should handle the following error scenarios:

1. **Connection Errors**: Network timeouts, DNS failures, unreachable hosts
2. **Authentication Errors**: Invalid credentials (401 Unauthorized)
3. **Authorization Errors**: Insufficient permissions (403 Forbidden)
4. **Resource Not Found**: Invalid endpoints or PDU IDs (404 Not Found)
5. **Server Errors**: PDU internal errors (500 Internal Server Error)
6. **TLS Errors**: Certificate validation failures

### Recommended Error Handling Pattern

```go
func (c *Client) GetWithRetry(endpoint string, maxRetries int) ([]byte, error) {
    var lastErr error
    
    for attempt := 0; attempt <= maxRetries; attempt++ {
        data, err := c.Get(endpoint)
        if err == nil {
            return data, nil
        }
        
        lastErr = err
        
        // Check if error is retryable (e.g., timeout, temporary network error)
        if !isRetryable(err) {
            return nil, err
        }
        
        if attempt < maxRetries {
            backoff := time.Duration(attempt+1) * time.Second
            c.logger.Warn("Request failed, retrying", "attempt", attempt+1, "backoff", backoff, "error", err)
            time.Sleep(backoff)
        }
    }
    

## Security Considerations

### TLS Configuration

Always use HTTPS in production:

```go
httpClient: &http.Client{
    Timeout: 10 * time.Second,
    Transport: &http.Transport{
        TLSClientConfig: &tls.Config{
            InsecureSkipVerify: false,        // Never skip in production
            MinVersion:         tls.VersionTLS12,
            CipherSuites: []uint16{
                tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
                tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
            },
        },
    },
}
```

### Credential Protection

- Never log credentials (username/password/authToken)
- Store credentials in Kubernetes Secrets or Vault
- Rotate credentials regularly
- Use short-lived session tokens

### Request Validation

- Validate all input endpoints before making requests
- Sanitize PDU IDs to prevent injection attacks
- Implement request timeouts to prevent resource exhaustion

## Usage Example

```go
package main

import (
    "encoding/json"
    "fmt"
    "log"
)

func main() {
    // Create client
    client, err := NewClient(
        "https://pdu01.datacenter.example.com",
        "admin",
        "secure-password",
    )
    if err != nil {
        log.Fatalf("Failed to create client: %v", err)
    }
    
    // Authenticate
    if err := client.Login(); err != nil {
        log.Fatalf("Failed to login: %v", err)
    }
    defer client.Logout()
    
    // Get PDU metrics
    metricsData, err := client.GetPDUMetrics("1")
    if err != nil {
        log.Fatalf("Failed to get PDU metrics: %v", err)
    }
    
    // Parse response
    var metrics map[string]interface{}
    if err := json.Unmarshal(metricsData, &metrics); err != nil {
        log.Fatalf("Failed to parse metrics: %v", err)
    }
    
    fmt.Printf("PDU Metrics: %+v\n", metrics)
}
```

## References

- [Redfish Specification](https://www.dmtf.org/standards/redfish)
- [Redfish API Documentation](https://redfish.dmtf.org/schemas/)
- [Panduit PDU Redfish Implementation Guide](https://www.panduit.com/)
- [DMTF Redfish Mock Server](https://github.com/DMTF/Redfish-Mockup-Server)

    return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}
```

```
