package salamoonder

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
)

var akamaiLogger = GetLogger("salamoonder.utils.akamai")

var akamaiScriptRe = regexp.MustCompile(`<script type="text/javascript".*?src="((/[0-9A-Za-z\-\_]+)+)">`)
var sbsdScriptRe = regexp.MustCompile(`(?i)<script[^>]+src=["']([^"']*/\.well-known/sbsd/[^"']+)["']`)

type AkamaiWeb struct {
	Client *SalamoonderSession
}

func NewAkamaiWeb(client *SalamoonderSession) *AkamaiWeb {
	return &AkamaiWeb{Client: client}
}

func (a *AkamaiWeb) getAkamaiURL(html, websiteURL string) (string, string) {
	match := akamaiScriptRe.FindStringSubmatch(html)
	if match == nil || len(match) < 2 {
		akamaiLogger.Warning("Failed to extract Akamai URL path from HTML")
		return "", ""
	}

	akamaiURLPath := match[1]

	parsed, err := url.Parse(websiteURL)
	if err != nil {
		akamaiLogger.Warning("Failed to parse website URL: %s", err)
		return "", ""
	}
	baseURL := fmt.Sprintf("%s://%s", parsed.Scheme, parsed.Hostname())

	resolved, err := url.Parse(akamaiURLPath)
	if err != nil {
		return "", ""
	}
	base, _ := url.Parse(baseURL)
	akamaiURL := base.ResolveReference(resolved).String()

	akamaiLogger.Debug("Extracted Akamai URL: %s", akamaiURL)
	return baseURL, akamaiURL
}

func (a *AkamaiWeb) FetchAndExtract(websiteURL, userAgent string, proxy string) (map[string]string, error) {
	akamaiLogger.Info("Starting Akamai extraction for: %s", websiteURL)
	a.Client.ClearHeaders()

	secChUa := ExtractSecChUa(userAgent)
	akamaiLogger.Debug("Generated sec-ch-ua: %s", secChUa)

	headers := map[string]string{
		"sec-ch-ua":                 secChUa,
		"sec-ch-ua-mobile":          "?0",
		"sec-ch-ua-platform":        `"Windows"`,
		"upgrade-insecure-requests": "1",
		"user-agent":                userAgent,
		"accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7",
		"sec-fetch-site":            "none",
		"sec-fetch-mode":            "navigate",
		"sec-fetch-user":            "?1",
		"sec-fetch-dest":            "document",
		"accept-language":           "en-US,en;q=0.9",
		"priority":                  "u=0, i",
	}

	akamaiLogger.Info("Fetching initial page...")
	resp, err := a.Client.Get(websiteURL, &RequestOptions{Headers: headers, Proxy: proxy})
	if err != nil {
		akamaiLogger.Error("Initial request failed: %s", err)
		return nil, err
	}

	if resp.StatusCode != 200 {
		akamaiLogger.Error("Initial request failed with status %d: %s", resp.StatusCode, resp.Text)
		return nil, nil
	}

	baseURL, akamaiURL := a.getAkamaiURL(resp.Text, websiteURL)
	if akamaiURL == "" {
		akamaiLogger.Error("Failed to parse Akamai URL from response")
		return nil, nil
	}
	akamaiLogger.Info("Akamai URL: %s", akamaiURL)

	abck := resp.Cookies.Get("_abck")
	if abck == "" {
		akamaiLogger.Error("_abck cookie not found in initial response")
		return nil, nil
	}
	akamaiLogger.Debug("Found _abck cookie: %s...", truncate(abck, 50))

	headers["referer"] = websiteURL
	headers["origin"] = baseURL
	headers["accept"] = "*/*"
	headers["sec-fetch-site"] = "same-origin"
	headers["sec-fetch-dest"] = "script"
	headers["sec-fetch-mode"] = "no-cors"
	delete(headers, "upgrade-insecure-requests")
	delete(headers, "sec-fetch-user")
	delete(headers, "priority")

	akamaiLogger.Info("Fetching Akamai script...")
	scriptResp, err := a.Client.Get(akamaiURL, &RequestOptions{Headers: headers, Proxy: proxy})
	if err != nil {
		akamaiLogger.Error("Script fetch failed: %s", err)
		return nil, err
	}

	if scriptResp.StatusCode != 200 {
		akamaiLogger.Error("Script fetch failed with status %d", scriptResp.StatusCode)
		return nil, nil
	}

	bmSz := a.Client.Cookies.Get("bm_sz")
	if bmSz == "" {
		akamaiLogger.Error("bm_sz cookie not found")
		return nil, nil
	}

	akamaiLogger.Info("Successfully extracted all Akamai data")
	akamaiLogger.Debug("bm_sz: %s", bmSz)
	akamaiLogger.Debug("Script data length: %d bytes", len(scriptResp.Text))

	return map[string]string{
		"base_url":    baseURL,
		"akamai_url":  akamaiURL,
		"script_data": scriptResp.Text,
		"abck":        abck,
		"bm_sz":       bmSz,
	}, nil
}

func (a *AkamaiWeb) PostSensor(akamaiURL, sensorData, userAgent, websiteURL string, proxy string) (map[string]string, error) {
	akamaiLogger.Info("Posting sensor data to Akamai endpoint")
	akamaiLogger.Debug("Current session cookies: %s", mapToJSON(a.Client.Cookies.GetDict()))

	secChUa := ExtractSecChUa(userAgent)

	parsed, _ := url.Parse(websiteURL)
	baseURL := fmt.Sprintf("%s://%s", parsed.Scheme, parsed.Hostname())

	headers := map[string]string{
		"sec-ch-ua":          secChUa,
		"sec-ch-ua-mobile":   "?0",
		"sec-ch-ua-platform": `"Windows"`,
		"user-agent":         userAgent,
		"content-type":       "application/json",
		"accept":             "*/*",
		"origin":             baseURL,
		"referer":            websiteURL,
		"sec-fetch-site":     "same-origin",
		"sec-fetch-mode":     "cors",
		"sec-fetch-dest":     "empty",
		"accept-language":    "en-US,en;q=0.9",
	}

	payload := map[string]interface{}{"sensor_data": sensorData}
	payloadJSON, _ := json.Marshal(payload)
	akamaiLogger.Debug("Posting sensor data, payload size: %d bytes", len(payloadJSON))

	resp, err := a.Client.Post(akamaiURL, &RequestOptions{
		Headers: headers,
		JSON:    payload,
		Proxy:   proxy,
	})
	if err != nil {
		akamaiLogger.Error("Sensor post failed: %s", err)
		return nil, err
	}

	if resp.StatusCode != 201 {
		akamaiLogger.Error("Sensor post failed with status %d: %s", resp.StatusCode, truncate(resp.Text, 200))
		var jsonResp map[string]interface{}
		if json.Unmarshal([]byte(resp.Text), &jsonResp) == nil {
			if jsonResp["success"] == "false" {
				akamaiLogger.Error("Response indicates failure: %s", resp.Text)
				return nil, nil
			}
		}
		return nil, nil
	}

	akamaiLogger.Info("Sensor post response: status=%d", resp.StatusCode)

	abck := resp.Cookies.Get("_abck")
	if abck == "" {
		akamaiLogger.Warning("No updated _abck cookie found in response")
		return nil, nil
	}

	bmSz := resp.Cookies.Get("bm_sz")
	if bmSz == "" {
		bmSz = a.Client.Cookies.Get("bm_sz")
	}

	akamaiLogger.Info("Successfully posted sensor data and received updated _abck")
	akamaiLogger.Debug("Updated _abck: %s...", truncate(abck, 50))
	akamaiLogger.Debug("Session cookies after request: %s", mapToJSON(a.Client.Cookies.GetDict()))

	return map[string]string{
		"_abck":  abck,
		"bm_sz":  bmSz,
		"cookies": mapToJSON(a.Client.Cookies.GetDict()),
	}, nil
}

type AkamaiSBSD struct {
	Client *SalamoonderSession
}

func NewAkamaiSBSD(client *SalamoonderSession) *AkamaiSBSD {
	return &AkamaiSBSD{Client: client}
}

func (a *AkamaiSBSD) getSbsdURL(html, baseURL string) string {
	match := sbsdScriptRe.FindStringSubmatch(html)
	if match == nil || len(match) < 2 {
		return ""
	}
	resolved, err := url.Parse(match[1])
	if err != nil {
		return ""
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	return base.ResolveReference(resolved).String()
}

func (a *AkamaiSBSD) FetchAndExtract(websiteURL, userAgent string, proxy string) (map[string]string, error) {
	akamaiLogger.Info("Starting SBSD extraction for: %s", websiteURL)
	a.Client.ClearHeaders()

	secChUa := ExtractSecChUa(userAgent)

	headers := map[string]string{
		"sec-ch-ua":                 secChUa,
		"sec-ch-ua-mobile":          "?0",
		"sec-ch-ua-platform":        `"Windows"`,
		"upgrade-insecure-requests": "1",
		"user-agent":                userAgent,
		"accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		"sec-fetch-site":            "none",
		"sec-fetch-mode":            "navigate",
		"sec-fetch-user":            "?1",
		"sec-fetch-dest":            "document",
		"accept-language":           "en-US,en;q=0.9",
		"priority":                  "u=0, i",
	}

	akamaiLogger.Info("Fetching initial page...")
	resp, err := a.Client.Get(websiteURL, &RequestOptions{Headers: headers, Proxy: proxy})
	if err != nil {
		akamaiLogger.Error("Initial request failed: %s", err)
		return nil, err
	}

	if resp.StatusCode != 200 {
		akamaiLogger.Error("Initial request failed: %d", resp.StatusCode)
		return nil, nil
	}

	parsed, _ := url.Parse(websiteURL)
	baseURL := fmt.Sprintf("%s://%s", parsed.Scheme, parsed.Hostname())

	sbsdURL := a.getSbsdURL(resp.Text, baseURL)
	if sbsdURL == "" {
		akamaiLogger.Error("Failed to parse SBSD URL")
		return nil, nil
	}
	akamaiLogger.Info("SBSD URL: %s", sbsdURL)

	headers["referer"] = websiteURL
	headers["origin"] = baseURL
	headers["accept"] = "*/*"
	headers["sec-fetch-site"] = "same-origin"
	headers["sec-fetch-dest"] = "script"
	headers["sec-fetch-mode"] = "no-cors"
	delete(headers, "upgrade-insecure-requests")
	delete(headers, "sec-fetch-user")
	delete(headers, "priority")

	akamaiLogger.Info("Fetching SBSD script...")
	scriptResp, err := a.Client.Get(sbsdURL, &RequestOptions{Headers: headers, Proxy: proxy})
	if err != nil {
		akamaiLogger.Error("SBSD script fetch failed: %s", err)
		return nil, err
	}

	if scriptResp.StatusCode != 200 {
		akamaiLogger.Error("SBSD script fetch failed: %d", scriptResp.StatusCode)
		return nil, nil
	}

	bmSo := a.Client.Cookies.Get("bm_so")
	sbsdO := a.Client.Cookies.Get("sbsd_o")

	var cookieName, cookieValue string
	if bmSo != "" {
		cookieName = "bm_so"
		cookieValue = bmSo
	} else if sbsdO != "" {
		cookieName = "sbsd_o"
		cookieValue = sbsdO
	} else {
		akamaiLogger.Error("Neither bm_so nor sbsd_o cookie found")
		return nil, nil
	}

	akamaiLogger.Info("Successfully extracted SBSD data")
	akamaiLogger.Debug("Using cookie: %s", cookieName)
	akamaiLogger.Debug("Script data length: %d bytes", len(scriptResp.Text))

	return map[string]string{
		"base_url":     baseURL,
		"sbsd_url":     sbsdURL,
		"script_data":  scriptResp.Text,
		"cookie_name":  cookieName,
		"cookie_value": cookieValue,
	}, nil
}

func (a *AkamaiSBSD) PostSBSD(sbsdPayload, postURL, userAgent, websiteURL string, proxy string) (map[string]string, error) {
	akamaiLogger.Info("Posting SBSD payload")

	decoded, err := base64DecodeString(sbsdPayload)
	if err != nil {
		akamaiLogger.Error("SBSD payload decode failed: %s", err)
		return nil, nil
	}

	secChUa := ExtractSecChUa(userAgent)

	parsed, _ := url.Parse(websiteURL)
	baseURL := fmt.Sprintf("%s://%s", parsed.Scheme, parsed.Hostname())

	parsedPost, _ := url.Parse(postURL)
	cleanPostURL := fmt.Sprintf("%s://%s%s", parsedPost.Scheme, parsedPost.Hostname(), parsedPost.Path)

	headers := map[string]string{
		"sec-ch-ua":          secChUa,
		"sec-ch-ua-mobile":   "?0",
		"sec-ch-ua-platform": `"Windows"`,
		"user-agent":         userAgent,
		"content-type":       "application/json",
		"accept":             "*/*",
		"origin":             baseURL,
		"referer":            websiteURL,
		"sec-fetch-site":     "same-origin",
		"sec-fetch-mode":     "cors",
		"sec-fetch-dest":     "empty",
		"accept-language":    "en-US,en;q=0.9",
		"priority":           "u=1, i",
	}

	akamaiLogger.Debug("SBSD post payload size: %d bytes", len(decoded))
	akamaiLogger.Debug("SBSD post URL: %s", cleanPostURL)

	body := map[string]interface{}{"body": string(decoded)}

	resp, err := a.Client.Post(cleanPostURL, &RequestOptions{
		Headers: headers,
		JSON:    body,
		Proxy:   proxy,
	})
	if err != nil {
		akamaiLogger.Error("SBSD post failed: %s", err)
		return nil, err
	}

	akamaiLogger.Info("SBSD response status: %d", resp.StatusCode)

	if resp.StatusCode != 200 {
		akamaiLogger.Error("SBSD post failed: %s", truncate(resp.Text, 200))
		return nil, nil
	}

	cookies := a.Client.Cookies.GetDict()
	if len(cookies) == 0 {
		akamaiLogger.Warning("No cookies set after SBSD post")
		return nil, nil
	}

	akamaiLogger.Info("SBSD post succeeded")
	akamaiLogger.Debug("Session cookies: %s", mapToJSON(cookies))

	return cookies, nil
}

func mapToJSON(m map[string]string) string {
	b, _ := json.Marshal(m)
	return string(b)
}

func base64DecodeString(s string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		decoded, err = base64.URLEncoding.DecodeString(s)
		if err != nil {
			decoded, err = base64.RawStdEncoding.DecodeString(s)
			if err != nil {
				return nil, err
			}
		}
	}
	return decoded, nil
}
