# Salamoonder SDK

A straightforward Go wrapper for Salamoonder's API, designed for easy integration and efficient usage. Perfect for solving captchas and bypassing bot detection on various platforms.

[![Go Version](https://img.shields.io/badge/go-1.21+-00ADD8.svg)](https://golang.org/doc/devel/release.html)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

## Features

- Simple and intuitive API
- Support for multiple captcha types:
  - Akamai Web Sensor
  - Akamai SBSD (Sensor Based Script Detection)
  - Kasada protection bypass
  - DataDome (Slider & Interstitial)
  - Incapsula/Imperva
- Built-in TLS fingerprinting with Chrome profiles
- Comprehensive error handling and logging
- Modern Go with proper error handling
- Production-ready with proper session management
- Concurrent task processing support

## Installation

```bash
go get github.com/salamoonder/salamoonder-go
```

## Requirements

- Go >= 1.21

## Quick Start

```go
package main

import (
    "fmt"
    "log"
    
    "github.com/salamoonder/salamoonder-go"
)

func main() {
    client := salamoonder.New("YOUR_API_KEY")
    
    // Create and solve a Kasada captcha task
    taskID, err := client.Task.CreateTask("KasadaCaptchaSolver", map[string]interface{}{
        "pjs_url": "https://example.com/script.js",
        "cd_only": "false",
    })
    if err != nil {
        log.Fatal(err)
    }
    
    // Poll for the result
    solution, err := client.Task.GetTaskResult(taskID)
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Println("Solution:", solution)
}
```

## Usage Examples

### Akamai Web Sensor

```go
// Configuration
const (
    URL        = "https://example.com/"
    USER_AGENT = `Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36`
    API_KEY    = "sr-YOUR-KEY"
)

client := salamoonder.New(API_KEY)

akamaiData, err := client.Akamai.FetchAndExtract(URL, USER_AGENT, PROXY)
if err != nil {
    log.Printf("ERROR: Failed to retrieve Akamai data: %v", err)
    return
}

// Solve 3 sensors (requires 3 API calls, you pay per sensor)
data := ""
for i := 0; i < 3; i++ {
    taskID, err := client.Task.CreateTask("AkamaiWebSensorSolver", map[string]interface{}{
        "url":        akamaiData["base_url"],
        "abck":       akamaiData["abck"],
        "bmsz":       akamaiData["bm_sz"],
        "script":     akamaiData["script_data"],
        "sensor_url": akamaiData["akamai_url"],
        "user_agent": USER_AGENT,
        "count":      i,
        "data":       data,
    })
    
    result, err := client.Task.GetTaskResult(taskID)
    payload := result["payload"].(string)
    data = result["data"].(string)
    
    cookie, err := client.Akamai.PostSensor(
        akamaiData["akamai_url"].(string),
        payload, USER_AGENT, URL, PROXY,
    )
}

log.Printf("SUCCESS: Successfully solved Akamai on %s", URL)
```

### Kasada

```go
// Kasada Captcha Challenge (Login Example)
client := salamoonder.New("sr-YOUR-KEY")

taskID, err := client.Task.CreateTask("KasadaCaptchaSolver", map[string]interface{}{
    "pjs_url": "https://example.com/149e9513-01fa-4fb0-aad4-566afd725d1b/2d206a39-8ed7-437e-a3be-862e0f06eea3/p.js",
    "cd_only": "false",
})

result, err := client.Task.GetTaskResult(taskID)

headers := map[string]string{
    "x-kpsdk-cd":     result["x-kpsdk-cd"].(string),
    "x-kpsdk-ct":     result["x-kpsdk-ct"].(string),
    "user-agent":     result["user-agent"].(string),
    "content-type":   "application/json",
}

payload := map[string]interface{}{
    "UserName": "USERNAME",
    "Password": "PASSWORD",
}

response, err := client.PostJSON("https://example.com/auth/v2/customer/login", payload, headers, "")

// Kasada Payload  
data, err := client.Kasada.ParseKasadaScript(scriptURL, USER_AGENT, PROXY)
taskID, err = client.Task.CreateTask("KasadaPayloadSolver", map[string]interface{}{
    "url":            "https://example.com",
    "script_url":     data["script_url"],
    "script_content": data["script_content"],
})
result, err = client.Task.GetTaskResult(taskID)
postSolution, err := client.Kasada.PostPayload("https://example.com", result, USER_AGENT, PROXY, false)
```

### DataDome

```go
client := salamoonder.New("sr-YOUR-KEY")

// Slider captcha - Full workflow
headers := map[string]string{"User-Agent": USER_AGENT}
response, err := client.Get(URL, headers, PROXY)
cookies := response.Cookies["datadome"]

if cookies == "" {
    log.Println("No DataDome cookie found")
    return
}

constructedURL, err := client.Datadome.ParseSliderURL(string(response.Body), cookies, URL)

taskID, err := client.Task.CreateTask("DataDomeSliderSolver", map[string]interface{}{
    "captcha_url":  constructedURL,
    "user_agent":   USER_AGENT,
    "country_code": "ch",
})

result, err := client.Task.GetTaskResult(taskID)

// Parse solved cookie
cookieStr := result["cookie"].(string)
solvedCookie := strings.Split(strings.Split(cookieStr, "datadome=")[1], ";")[0]

client.SessionCookies.Set("datadome", solvedCookie, ".example.com")

// Validate bypass
response, err = client.Get(URL, headers, "")
if response.StatusCode == 200 {
    log.Printf("SUCCESS: Successfully bypassed DD Slider")
}

// Interstitial follows same pattern with ParseInterstitialURL
```

### Direct Client Methods

For custom HTTP requests with TLS client impersonation:

```go
client := salamoonder.New("YOUR_API_KEY")

// GET request
response, err := client.Get("https://example.com", 
    map[string]string{"User-Agent": "Custom UA"},
    "http://proxy:port",
)
if err != nil {
    log.Fatal(err)
}

// POST request
postResponse, err := client.Post("https://example.com/api",
    []byte(`{"key": "value"}`),
    map[string]string{"Content-Type": "application/json"},
    "",
)
if err != nil {
    log.Fatal(err)
}
```

## API Reference

### Salamoonder Struct

Main entry point for the library.

```go
client := salamoonder.New("api_key")
```

**Parameters:**
- `apiKey` (string) - Your Salamoonder API key (required)

**Properties:**
- `Task` - Tasks API instance (recommended for solving captchas)
- `Akamai`, `AkamaiSBSD`, `Datadome`, `Kasada` - Low-level solver instances (advanced use only)
- `SessionCookies` - Session information and cookies

**Methods:**
- `Get(url, headers, proxy)` - Make a GET request
- `Post(url, body, headers, proxy)` - Make a POST request
- `PostBytes(url, body, headers, proxy)` - Make a POST request with binary body

## Supported Anti-Bot Solutions

### 🔒 Kasada
- Script extraction and parsing
- Payload solving  
- Challenge submission

### 🛡️ Akamai Bot Manager
- Web sensor data extraction
- SBSD (Sensor Based Script Detection) support
- Cookie management

### 🔐 DataDome
- Slider captcha URL parsing
- Interstitial challenge support

### 🔒 Incapsula/Imperva
- Reese84 challenge solving
- UTMVC cookie generation

## Supported Captcha Types

- `KasadaCaptchaSolver`
- `KasadaPayloadSolver`
- `AkamaiWebSensorSolver`
- `AkamaiSBSDSolver`
- `DataDomeSliderSolver`
- `DataDomeInterstitialSolver`
- `IncapsulaReese84Solver`
- `IncapsulaUTMVCSolver`

## Module Exports

You can import individual components if needed:

```go
// Main class and main exports
import "github.com/salamoonder/salamoonder-go"

// Error types
// APIError and MissingAPIKeyError are exported from the main package

// For advanced/low-level operations only
// All utility structs (Akamai, AkamaiSBSD, Datadome, Kasada) are accessible
// through the main client instance
```

**Note:** The utility instances (`Akamai`, `AkamaiSBSD`, `Datadome`, `Kasada`) are for low-level operations. For most use cases, use the Tasks API through the main client.

## Error Handling

```go
import (
    "fmt"
    "log"
    
    "github.com/salamoonder/salamoonder-go"
)

func main() {
    client := salamoonder.New("YOUR_API_KEY")
    
    taskID, err := client.Task.CreateTask("KasadaCaptchaSolver", map[string]interface{}{
        "pjs_url": "https://example.com/script.js",
        "cd_only": "false",
    })
    
    if err != nil {
        if apiErr, ok := err.(*salamoonder.APIError); ok {
            fmt.Printf("API error: %s (Code: %d)\n", apiErr.Message, apiErr.StatusCode)
        } else if _, ok := err.(*salamoonder.MissingAPIKeyError); ok {
            fmt.Println("API key is required")
        } else {
            fmt.Printf("Unexpected error: %v\n", err)
        }
        return
    }
    
    result, err := client.Task.GetTaskResult(taskID)
    if err != nil {
        fmt.Printf("Task failed: %v\n", err)
        return
    }
    
    fmt.Printf("Success: %+v\n", result)
}
```

## Configuration

### Proxy Support
```go
// All methods support proxy parameter
result, err := client.Akamai.FetchAndExtract(
    "https://example.com",
    "Mozilla/5.0...",
    "http://username:password@proxy.example.com:8080",
)
```

## Logging

Enable debug logging to see detailed information:

```go
import "log"

// The SDK uses structured logging with timestamps
// All operations are logged automatically with appropriate levels
// Format: YYYY-MM-DD HH:MM:SS - name - LEVEL - message
```

## Performance Tips

- Reuse the same client instance for multiple operations
- Process multiple tasks concurrently with goroutines
- Implement proper error handling and retries

```go
// Good: Reuse client for multiple operations and process concurrently
client := salamoonder.New("YOUR_API_KEY")

var wg sync.WaitGroup
results := make(chan map[string]interface{}, len(taskIDs))

for _, taskID := range taskIDs {
    wg.Add(1)
    go func(id string) {
        defer wg.Done()
        result, err := client.Task.GetTaskResult(id)
        if err == nil {
            results <- result
        }
    }(taskID)
}

wg.Wait()
close(results)

// Process results
for result := range results {
    fmt.Printf("Result: %+v\n", result)
}
```

## License

MIT - See [LICENSE](LICENSE) file for details

## Support

For issues, feature requests, or questions, please visit:
- **Website**: [salamoonder.com](https://salamoonder.com)
- **Documentation**: [salamoonder.com/docs](https://apidocs.salamoonder.com)
- **Support**: [support@salamoonder.com](mailto:support@salamoonder.com)
- **Telegram**: [Text us!](https://t.me/salamoonder_bot)