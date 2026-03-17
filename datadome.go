package salamoonder

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

var datadomeLogger = GetLogger("salamoonder.utils.datadome")

type Datadome struct {
	Client *SalamoonderSession
}

func NewDatadome(client *SalamoonderSession) *Datadome {
	return &Datadome{Client: client}
}

func parseDDObject(html string) (map[string]interface{}, error) {
	parts := strings.SplitN(html, "var dd=", 2)
	if len(parts) < 2 {
		return nil, fmt.Errorf("var dd= not found in HTML")
	}
	rest := strings.SplitN(parts[1], "</script>", 2)
	if len(rest) < 1 {
		return nil, fmt.Errorf("closing </script> not found")
	}
	jsObject := strings.ReplaceAll(rest[0], "'", `"`)
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(jsObject), &parsed); err != nil {
		return nil, err
	}
	return parsed, nil
}

func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

func (d *Datadome) ParseSliderURL(html, dataDomeCookie, referer string) (string, error) {
	datadomeLogger.Info("Parsing DataDome slider URL from HTML")

	parsed, err := parseDDObject(html)
	if err != nil {
		datadomeLogger.Error("Failed to parse object: %s", err)
		return "", fmt.Errorf("failed to parse object")
	}
	datadomeLogger.Debug("Successfully parsed object")

	if toString(parsed["t"]) == "bv" {
		datadomeLogger.Error("IP is blocked (t=bv), exiting...")
		return "", fmt.Errorf("IP is blocked (t=bv)")
	}

	params := url.Values{}
	params.Set("initialCid", toString(parsed["cid"]))
	params.Set("hash", toString(parsed["hsh"]))
	params.Set("cid", dataDomeCookie)
	params.Set("t", toString(parsed["t"]))
	params.Set("referer", referer)
	params.Set("s", toString(parsed["s"]))
	params.Set("e", toString(parsed["e"]))
	params.Set("dm", "cd")

	captchaURL := fmt.Sprintf("https://geo.captcha-delivery.com/captcha/?%s", params.Encode())
	datadomeLogger.Info("Constructed slider URL: %s...", truncate(captchaURL, 80))
	return captchaURL, nil
}

func (d *Datadome) ParseInterstitialURL(html, dataDomeCookie, referer string) (string, error) {
	datadomeLogger.Info("Parsing DataDome interstitial URL from HTML")

	parsed, err := parseDDObject(html)
	if err != nil {
		datadomeLogger.Error("Failed to parse object: %s", err)
		return "", fmt.Errorf("failed to parse object")
	}
	datadomeLogger.Debug("Successfully parsed object")

	params := url.Values{}
	params.Set("initialCid", toString(parsed["cid"]))
	params.Set("hash", toString(parsed["hsh"]))
	params.Set("cid", dataDomeCookie)
	params.Set("referer", referer)
	params.Set("s", toString(parsed["s"]))
	params.Set("e", toString(parsed["e"]))
	params.Set("b", toString(parsed["b"]))
	params.Set("dm", "cd")

	interstitialURL := fmt.Sprintf("https://geo.captcha-delivery.com/interstitial/?%s", params.Encode())
	datadomeLogger.Info("Constructed interstitial URL: %s...", truncate(interstitialURL, 80))
	return interstitialURL, nil
}
