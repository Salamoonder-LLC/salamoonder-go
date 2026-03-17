package salamoonder

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var kasadaLogger = GetLogger("salamoonder.utils.kasada")

var (
	externalScriptRe = regexp.MustCompile(`<script\s+src=["']([^"']+)["']`)
	inlineScriptRe   = regexp.MustCompile(`(?s)<script[^>]*>(.*?)</script>`)
	ampRe            = regexp.MustCompile(`&amp;`)
)

type Kasada struct {
	Client *SalamoonderSession
}

func NewKasada(client *SalamoonderSession) *Kasada {
	return &Kasada{Client: client}
}

type scriptResult struct {
	Type    string
	Content string
	URLs    []string
}

func (k *Kasada) getScriptURL(html, baseURL string) *scriptResult {
	externalMatches := externalScriptRe.FindAllStringSubmatch(html, -1)
	scriptURLs := make([]string, 0)
	for _, m := range externalMatches {
		if len(m) > 1 {
			scriptURLs = append(scriptURLs, m[1])
		}
	}

	inlineMatches := inlineScriptRe.FindAllStringSubmatch(html, -1)
	kasadaLogger.Debug("Found %d external and %d inline scripts", len(scriptURLs), len(inlineMatches))

	for _, m := range inlineMatches {
		if len(m) < 2 {
			continue
		}
		content := m[1]
		if strings.Contains(content, "KPSDK.scriptStart") || strings.Contains(content, "ips.js") {
			kasadaLogger.Debug("Found inline Kasada script: %d bytes", len(content))
			return &scriptResult{Type: "inline", Content: strings.TrimSpace(content)}
		}
	}

	resolvedURLs := make([]string, 0, len(scriptURLs))
	for _, src := range scriptURLs {
		src = ampRe.ReplaceAllString(src, "&")
		if !strings.HasPrefix(src, "http") {
			src = strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(src, "/")
		}
		resolvedURLs = append(resolvedURLs, src)
	}

	kasadaLogger.Debug("Resolved %d external script URLs", len(resolvedURLs))
	return &scriptResult{Type: "external", URLs: resolvedURLs}
}

func (k *Kasada) ParseKasadaScript(rawURL, userAgent string, proxy string) (map[string]string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	kasadaLogger.Info("Starting Kasada script extraction for: %s", parsed.Hostname())

	if !parsed.Query().Has("x-kpsdk-v") {
		kasadaLogger.Warning("x-kpsdk-v parameter not found in URL")
		return nil, nil
	}

	k.Client.ClearHeaders()
	baseURL := fmt.Sprintf("https://%s", parsed.Hostname())

	headers := map[string]string{
		"sec-ch-ua":                 ExtractSecChUa(userAgent),
		"sec-ch-ua-mobile":          "?0",
		"sec-ch-ua-platform":        `"Windows"`,
		"upgrade-insecure-requests": "1",
		"user-agent":                userAgent,
		"accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7",
		"sec-fetch-site":            "same-site",
		"sec-fetch-mode":            "navigate",
		"sec-fetch-dest":            "iframe",
		"accept-language":           "en-US,en;q=0.9",
		"referer":                   baseURL + "/",
		"priority":                  "u=0, i",
	}

	kasadaLogger.Info("Fetching fingerprint endpoint...")
	resp, err := k.Client.Get(rawURL, &RequestOptions{Headers: headers, Proxy: proxy})
	if err != nil {
		kasadaLogger.Error("Fingerprint request failed: %s", err)
		return nil, err
	}

	if resp.StatusCode != 429 && resp.StatusCode != 200 {
		kasadaLogger.Warning("Expected 429 or 200 status code, got %d", resp.StatusCode)
		return nil, nil
	}

	scriptData := k.getScriptURL(resp.Text, baseURL)

	scriptsContent := ""
	scriptURL := ""

	if scriptData.Type == "inline" {
		scriptsContent = scriptData.Content
		kasadaLogger.Info("Using inline Kasada script")
	} else {
		kasadaLogger.Info("Fetching external Kasada script(s), %d URLs to check", len(scriptData.URLs))
		for i, src := range scriptData.URLs {
			kasadaLogger.Debug("Fetching external script %d/%d: %s", i+1, len(scriptData.URLs), truncate(src, 80))
			scriptResp, err := k.Client.Get(src, &RequestOptions{Headers: headers, Proxy: proxy})
			if err != nil {
				continue
			}
			if strings.Contains(scriptResp.Text, "ips.js") || strings.Contains(scriptResp.Text, "KPSDK.scriptStart") {
				scriptsContent = scriptResp.Text
				scriptURL = scriptResp.URL
				kasadaLogger.Info("Successfully fetched Kasada script from URL: %s", truncate(src, 80))
				break
			}
		}
	}

	kasadaLogger.Debug("Final script size: %d bytes", len(scriptsContent))
	kasadaLogger.Info("Kasada extraction complete")

	return map[string]string{
		"script_content": `window.KPSDK={};KPSDK.now=typeof performance!=='undefined'&&performance.now?performance.now.bind(performance):Date.now.bind(Date);KPSDK.start=KPSDK.now(); ` + scriptsContent,
		"script_url":     scriptURL,
	}, nil
}

type KasadaSolution struct {
	Headers map[string]string `json:"headers"`
	Payload string            `json:"payload"`
}

type KasadaPostResult struct {
	Response map[string]interface{} `json:"response"`
	XKpsdkCT *string                `json:"x-kpsdk-ct"`
	XKpsdkR  *string                `json:"x-kpsdk-r"`
	XKpsdkST *string                `json:"x-kpsdk-st"`
	XKpsdkV  *string                `json:"x-kpsdk-v"`
	XKpsdkH  *string                `json:"x-kpsdk-h,omitempty"`
	XKpsdkFC *string                `json:"x-kpsdk-fc,omitempty"`
}

func (k *Kasada) PostPayload(rawURL string, solution *KasadaSolution, userAgent string, proxy string, mfc bool) (*KasadaPostResult, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	kasadaLogger.Info("Starting Kasada payload post for: %s", parsed.Hostname())
	k.Client.ClearHeaders()

	baseURL := fmt.Sprintf("https://%s", parsed.Hostname())
	tlURL := fmt.Sprintf("%s/149e9513-01fa-4fb0-aad4-566afd725d1b/2d206a39-8ed7-437e-a3be-862e0f06eea3/tl", baseURL)

	headers := map[string]string{
		"content-type":       "application/octet-stream",
		"sec-ch-ua-platform": `"Windows"`,
		"sec-ch-ua":          ExtractSecChUa(userAgent),
		"sec-ch-ua-mobile":   "?0",
		"user-agent":         userAgent,
		"accept":             "*/*",
		"origin":             baseURL,
		"sec-fetch-site":     "same-origin",
		"sec-fetch-mode":     "cors",
		"sec-fetch-dest":     "empty",
		"referer":            fmt.Sprintf("%s/149e9513-01fa-4fb0-aad4-566afd725d1b/2d206a39-8ed7-437e-a3be-862e0f06eea3/fp?x-kpsdk-v=%s", baseURL, solution.Headers["x-kpsdk-v"]),
		"accept-encoding":    "gzip, deflate, br, zstd",
		"accept-language":    "en-US,en;q=0.9",
		"priority":           "u=1, i",
		"x-kpsdk-ct":         solution.Headers["x-kpsdk-ct"],
		"x-kpsdk-dt":         solution.Headers["x-kpsdk-dt"],
		"x-kpsdk-im":         solution.Headers["x-kpsdk-im"],
		"x-kpsdk-h":          "01",
		"x-kpsdk-v":          solution.Headers["x-kpsdk-v"],
	}

	payloadBytes, err := base64.StdEncoding.DecodeString(solution.Payload)
	if err != nil {
		payloadBytes, err = base64.RawStdEncoding.DecodeString(solution.Payload)
		if err != nil {
			kasadaLogger.Error("Failed to decode payload: %s", err)
			return nil, fmt.Errorf("failed to decode payload: %w", err)
		}
	}
	kasadaLogger.Debug("Payload size: %d bytes", len(payloadBytes))

	kasadaLogger.Info("Posting payload to /tl endpoint...")
	resp, err := k.Client.PostBytes(tlURL, payloadBytes, headers, proxy)
	if err != nil {
		kasadaLogger.Error("Payload post failed: %s", err)
		return nil, err
	}

	kasadaLogger.Info("Payload post response: status=%d", resp.StatusCode)

	if resp.StatusCode != 200 {
		kasadaLogger.Warning("Unexpected response status: %d - %s", resp.StatusCode, truncate(resp.Text, 200))
		return nil, nil
	}

	var respJSON map[string]interface{}
	if err := json.Unmarshal([]byte(resp.Text), &respJSON); err != nil {
		kasadaLogger.Error("Failed to parse response JSON: %s", truncate(resp.Text, 200))
		return nil, nil
	}

	reload, ok := respJSON["reload"].(bool)
	if !ok || !reload {
		kasadaLogger.Error("Response missing or has reload!=true: %s", resp.Text)
		return nil, nil
	}

	kpsdkR, ok := resp.Headers["x-kpsdk-r"]
	if !ok || kpsdkR == "" {
		kasadaLogger.Error("Missing x-kpsdk-r header in response")
		return nil, nil
	}

	if kpsdkR == "1-AA" || kpsdkR == "1-AQ" {
		kasadaLogger.Error("Bad fingerprint or proxy detected: x-kpsdk-r=%s", kpsdkR)
		return nil, nil
	}

	result := &KasadaPostResult{
		Response: map[string]interface{}{
			"status_code": resp.StatusCode,
			"text":        resp.Text,
			"cookies":     k.Client.Cookies.GetDict(),
			"headers":     resp.Headers,
		},
		XKpsdkCT: getHeaderPtr(resp.Headers, "x-kpsdk-ct"),
		XKpsdkR:  getHeaderPtr(resp.Headers, "x-kpsdk-r"),
		XKpsdkST: getHeaderPtr(resp.Headers, "x-kpsdk-st"),
		XKpsdkV:  strPtr(solution.Headers["x-kpsdk-v"]),
	}

	if mfc {
		mfcURL := fmt.Sprintf("%s/149e9513-01fa-4fb0-aad4-566afd725d1b/2d206a39-8ed7-437e-a3be-862e0f06eea3/mfc", baseURL)
		mfcHeaders := map[string]string{
			"sec-ch-ua-platform": `"Windows"`,
			"x-kpsdk-h":         "01",
			"sec-ch-ua":         ExtractSecChUa(userAgent),
			"sec-ch-ua-mobile":  "?0",
			"x-kpsdk-v":         solution.Headers["x-kpsdk-v"],
			"user-agent":        userAgent,
			"accept":            "*/*",
			"sec-fetch-site":    "same-origin",
			"sec-fetch-mode":    "cors",
			"sec-fetch-dest":    "empty",
			"referer":           rawURL,
			"accept-encoding":   "gzip, deflate, br, zstd",
			"accept-language":   "en-US,en;q=0.9",
			"priority":          "u=1, i",
		}

		mfcResp, err := k.Client.Get(mfcURL, &RequestOptions{Headers: mfcHeaders, Proxy: proxy})
		if err == nil && mfcResp.StatusCode == 200 {
			result.XKpsdkH = getHeaderPtr(mfcResp.Headers, "x-kpsdk-h")
			result.XKpsdkFC = getHeaderPtr(mfcResp.Headers, "x-kpsdk-fc")
		}
	}

	return result, nil
}

func getHeaderPtr(headers map[string]string, key string) *string {
	if v, ok := headers[key]; ok {
		return &v
	}
	return nil
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
