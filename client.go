package salamoonder

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	http "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

var clientLogger = GetLogger("salamoonder.client")

type APIError struct {
	Message string
}

func (e *APIError) Error() string {
	return e.Message
}

type MissingAPIKeyError struct {
	Message string
}

func (e *MissingAPIKeyError) Error() string {
	return e.Message
}

type SessionCookies struct {
	cookies map[string]*cookieEntry
}

type cookieEntry struct {
	Name   string
	Value  string
	Domain string
	Path   string
}

func NewSessionCookies() *SessionCookies {
	return &SessionCookies{cookies: make(map[string]*cookieEntry)}
}

func (sc *SessionCookies) Set(name, value, domain, path string) {
	if path == "" {
		path = "/"
	}
	key := fmt.Sprintf("%s:%s:%s", domain, path, name)
	sc.cookies[key] = &cookieEntry{Name: name, Value: value, Domain: domain, Path: path}
}

func (sc *SessionCookies) Get(name string) string {
	for _, c := range sc.cookies {
		if c.Name == name {
			return c.Value
		}
	}
	return ""
}

func (sc *SessionCookies) GetDict() map[string]string {
	result := make(map[string]string)
	for _, c := range sc.cookies {
		result[c.Name] = c.Value
	}
	return result
}

func (sc *SessionCookies) GetDictForURL(rawURL string) map[string]string {
	result := make(map[string]string)
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return result
	}
	for _, c := range sc.cookies {
		if c.Domain == "" || strings.HasSuffix(parsed.Hostname(), strings.TrimPrefix(c.Domain, ".")) {
			result[c.Name] = c.Value
		}
	}
	return result
}

func (sc *SessionCookies) Clear() {
	sc.cookies = make(map[string]*cookieEntry)
}

type Response struct {
	StatusCode int
	Text       string
	Headers    map[string]string
	Cookies    *SessionCookies
	URL        string
}

func (r *Response) JSON(target interface{}) error {
	return json.Unmarshal([]byte(r.Text), target)
}

type RequestOptions struct {
	Headers     map[string]string
	Proxy       string
	JSON        interface{}
	Data        []byte
	Verify      *bool
	Impersonate string
}

type SalamoonderSession struct {
	APIKey      string
	BaseURL     string
	Impersonate string
	Headers     map[string]string
	Cookies     *SessionCookies
}

func NewSalamoonderSession(apiKey string, baseURL string, impersonate string) (*SalamoonderSession, error) {
	if strings.TrimSpace(apiKey) == "" {
		clientLogger.Error("Attempted to initialize client without API key")
		return nil, &MissingAPIKeyError{Message: "API key is required. Pass it when creating the client."}
	}

	if baseURL == "" {
		baseURL = "https://salamoonder.com/api"
	}
	if impersonate == "" {
		impersonate = "chrome_120"
	}

	clientLogger.Debug("Client initialized with base_url: %s, impersonate: %s", baseURL, impersonate)

	return &SalamoonderSession{
		APIKey:      apiKey,
		BaseURL:     baseURL,
		Impersonate: impersonate,
		Headers:     make(map[string]string),
		Cookies:     NewSessionCookies(),
	}, nil
}

func getTLSProfile(impersonate string) profiles.ClientProfile {
	switch impersonate {
	case "chrome_133", "chrome133a", "chrome_124", "chrome_120":
		return profiles.Chrome_120
	case "chrome_116":
		return profiles.Chrome_117
	case "chrome_117":
		return profiles.Chrome_117
	default:
		return profiles.Chrome_120
	}
}

func (s *SalamoonderSession) newTLSClient(opts *RequestOptions) (tls_client.HttpClient, error) {
	options := []tls_client.HttpClientOption{
		tls_client.WithTimeoutSeconds(30),
		tls_client.WithClientProfile(getTLSProfile(s.Impersonate)),
		tls_client.WithNotFollowRedirects(),
		tls_client.WithRandomTLSExtensionOrder(),
		tls_client.WithInsecureSkipVerify(),
	}

	if opts != nil && opts.Proxy != "" {
		options = append(options, tls_client.WithProxyUrl(opts.Proxy))
	}

	return tls_client.NewHttpClient(tls_client.NewNoopLogger(), options...)
}

func (s *SalamoonderSession) newBytesTLSClient(proxy string) (tls_client.HttpClient, error) {
	options := []tls_client.HttpClientOption{
		tls_client.WithTimeoutSeconds(30),
		tls_client.WithClientProfile(getTLSProfile(s.Impersonate)),
		tls_client.WithNotFollowRedirects(),
		tls_client.WithRandomTLSExtensionOrder(),
		tls_client.WithInsecureSkipVerify(),
		tls_client.WithForceHttp1(),
	}

	if proxy != "" {
		options = append(options, tls_client.WithProxyUrl(proxy))
	}

	return tls_client.NewHttpClient(tls_client.NewNoopLogger(), options...)
}

func (s *SalamoonderSession) extractCookies(resp *http.Response) {
	if resp == nil {
		return
	}
	for _, cookie := range resp.Cookies() {
		s.Cookies.Set(cookie.Name, cookie.Value, cookie.Domain, cookie.Path)
	}
}

func (s *SalamoonderSession) buildCookieHeader(rawURL string) string {
	cookieDict := s.Cookies.GetDictForURL(rawURL)
	if len(cookieDict) == 0 {
		return ""
	}
	parts := make([]string, 0, len(cookieDict))
	for k, v := range cookieDict {
		parts = append(parts, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(parts, "; ")
}

func (s *SalamoonderSession) executeRequest(method, rawURL string, opts *RequestOptions) (*Response, error) {
	client, err := s.newTLSClient(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create TLS client: %w", err)
	}

	headers := make(http.Header)
	for k, v := range s.Headers {
		headers.Set(k, v)
	}
	if opts != nil {
		for k, v := range opts.Headers {
			headers.Set(k, v)
		}
	}

	cookieStr := s.buildCookieHeader(rawURL)
	if cookieStr != "" {
		existing := headers.Get("cookie")
		if existing != "" {
			headers.Set("cookie", existing+"; "+cookieStr)
		} else {
			headers.Set("cookie", cookieStr)
		}
	}

	var req *http.Request
	if method == "GET" {
		req, err = http.NewRequest("GET", rawURL, nil)
	} else if method == "POST" {
		var bodyStr string
		if opts != nil && opts.JSON != nil {
			jsonBytes, jsonErr := json.Marshal(opts.JSON)
			if jsonErr != nil {
				return nil, fmt.Errorf("failed to marshal JSON: %w", jsonErr)
			}
			bodyStr = string(jsonBytes)
			if headers.Get("content-type") == "" {
				headers.Set("content-type", "application/json")
			}
		} else if opts != nil && opts.Data != nil {
			bodyStr = string(opts.Data)
		}
		req, err = http.NewRequest("POST", rawURL, strings.NewReader(bodyStr))
	} else {
		req, err = http.NewRequest(method, rawURL, nil)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header = headers

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	s.extractCookies(resp)

	flatHeaders := make(map[string]string)
	for k := range resp.Header {
		flatHeaders[strings.ToLower(k)] = resp.Header.Get(k)
	}

	return &Response{
		StatusCode: resp.StatusCode,
		Text:       string(bodyBytes),
		Headers:    flatHeaders,
		Cookies:    s.Cookies,
		URL:        rawURL,
	}, nil
}

func (s *SalamoonderSession) post(apiURL string, payload map[string]interface{}, proxy string) (map[string]interface{}, error) {
	if strings.TrimSpace(s.APIKey) == "" {
		clientLogger.Error("API key missing during request")
		return nil, &MissingAPIKeyError{Message: "API key is required"}
	}

	if proxy != "" {
		clientLogger.Debug("POST %s (via proxy: %s)", apiURL, proxy)
	} else {
		clientLogger.Debug("POST %s", apiURL)
	}

	body := map[string]interface{}{
		"api_key": s.APIKey,
	}
	for k, v := range payload {
		body[k] = v
	}

	resp, err := s.executeRequest("POST", apiURL, &RequestOptions{
		JSON:  body,
		Proxy: proxy,
	})
	if err != nil {
		return nil, err
	}

	var data map[string]interface{}
	if jsonErr := resp.JSON(&data); jsonErr != nil {
		clientLogger.Error("Invalid JSON response (status=%d)", resp.StatusCode)
		return nil, &APIError{Message: fmt.Sprintf("Invalid response from API (%d): %s", resp.StatusCode, truncate(resp.Text, 200))}
	}

	if resp.StatusCode >= 400 {
		msg := "Request failed"
		if v, ok := data["error_description"].(string); ok && v != "" {
			msg = v
		} else if v, ok := data["error"].(string); ok && v != "" {
			msg = v
		}
		clientLogger.Error("API error (status=%d): %s", resp.StatusCode, msg)
		return nil, &APIError{Message: msg}
	}

	clientLogger.Debug("Request successful (status=%d)", resp.StatusCode)
	return data, nil
}

func (s *SalamoonderSession) Get(rawURL string, opts *RequestOptions) (*Response, error) {
	if opts != nil && opts.Proxy != "" {
		clientLogger.Debug("GET %s (via proxy: %s)", rawURL, opts.Proxy)
	} else {
		clientLogger.Debug("GET %s", rawURL)
	}
	return s.executeRequest("GET", rawURL, opts)
}

func (s *SalamoonderSession) Post(rawURL string, opts *RequestOptions) (*Response, error) {
	if opts != nil && opts.Proxy != "" {
		clientLogger.Debug("POST %s (via proxy: %s)", rawURL, opts.Proxy)
	} else {
		clientLogger.Debug("POST %s", rawURL)
	}
	return s.executeRequest("POST", rawURL, opts)
}

func (s *SalamoonderSession) PostBytes(rawURL string, data []byte, headers map[string]string, proxy string) (*Response, error) {
	client, err := s.newBytesTLSClient(proxy)
	if err != nil {
		return nil, fmt.Errorf("failed to create byte TLS client: %w", err)
	}

	req, err := http.NewRequest("POST", rawURL, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	cookieStr := s.buildCookieHeader(rawURL)
	if cookieStr != "" {
		req.Header.Set("cookie", cookieStr)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	s.extractCookies(resp)

	flatHeaders := make(map[string]string)
	for k := range resp.Header {
		flatHeaders[strings.ToLower(k)] = resp.Header.Get(k)
	}

	return &Response{
		StatusCode: resp.StatusCode,
		Text:       string(bodyBytes),
		Headers:    flatHeaders,
		Cookies:    s.Cookies,
		URL:        rawURL,
	}, nil
}

func (s *SalamoonderSession) ClearHeaders() {
	s.Headers = make(map[string]string)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

func IsAPIError(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr)
}

func IsMissingAPIKeyError(err error) bool {
	var keyErr *MissingAPIKeyError
	return errors.As(err, &keyErr)
}
