package app

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

type HTTPService struct {
	client      *http.Client
	insecure    *http.Client
	errOut      io.Writer
	warnOnce    sync.Once
	oauthURL    string
	cloudAPI    string
	userInfoURL string
}

func NewHTTPService(errOut io.Writer) *HTTPService {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 32
	transport.MaxIdleConnsPerHost = 16
	transport.IdleConnTimeout = 90 * time.Second
	insecureTransport := transport.Clone()
	insecureTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // explicit user opt-in only
	return &HTTPService{
		client:      &http.Client{Transport: transport},
		insecure:    &http.Client{Transport: insecureTransport},
		errOut:      errOut,
		oauthURL:    oauthTokenURL,
		cloudAPI:    cloudCodeAPI,
		userInfoURL: "https://www.googleapis.com/oauth2/v1/userinfo",
	}
}

func allowInsecureTLS() bool {
	value := os.Getenv("AGY_SWAP_INSECURE_TLS")
	if value == "" {
		value = os.Getenv("AGY_SWAP_ALLOW_INSECURE_SSL")
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func (h *HTTPService) do(request *http.Request) (*http.Response, error) {
	response, err := h.client.Do(request)
	if err == nil || !allowInsecureTLS() {
		return response, err
	}
	clone := request.Clone(request.Context())
	if request.GetBody != nil {
		body, bodyErr := request.GetBody()
		if bodyErr != nil {
			return nil, err
		}
		clone.Body = body
	}
	h.warnOnce.Do(func() {
		_, _ = fmt.Fprintln(h.errOut, "Warning: AGY_SWAP_INSECURE_TLS is enabled; TLS certificate verification is disabled.")
	})
	return h.insecure.Do(clone)
}

func (h *HTTPService) jsonRequest(ctx context.Context, method, endpoint string, headers map[string]string, body any, timeout time.Duration, limit int64) (map[string]any, int, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		bodyReader = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, bodyReader)
	if err != nil {
		return nil, 0, err
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	if request.Header.Get("User-Agent") == "" {
		request.Header.Set("User-Agent", "Mozilla/5.0")
	}
	requestCtx, cancel := context.WithTimeout(request.Context(), timeout)
	defer cancel()
	request = request.WithContext(requestCtx)
	response, err := h.do(request)
	if err != nil {
		return nil, 0, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, response.StatusCode, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, response.StatusCode, err
	}
	if int64(len(data)) > limit {
		return nil, response.StatusCode, fmt.Errorf("response exceeds %d bytes", limit)
	}
	var result map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&result); err != nil {
		return nil, response.StatusCode, err
	}
	if result == nil {
		return nil, response.StatusCode, fmt.Errorf("invalid JSON object")
	}
	return result, response.StatusCode, nil
}

func decodeToken(value string) map[string]any {
	if value == "" {
		return nil
	}
	raw := []byte(value)
	if strings.HasPrefix(value, "go-keyring-base64:") {
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, "go-keyring-base64:"))
		if err != nil {
			return nil
		}
		raw = decoded
	}
	var result map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&result) != nil {
		return nil
	}
	return result
}

func tokenObject(token map[string]any) map[string]any {
	if token == nil {
		return nil
	}
	return getMap(token["token"])
}

func decodeJWTClaims(jwt string) map[string]any {
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		return nil
	}
	payload := parts[1]
	if rem := len(payload) % 4; rem != 0 {
		payload += strings.Repeat("=", 4-rem)
	}
	data, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		return nil
	}
	var claims map[string]any
	if json.Unmarshal(data, &claims) != nil {
		return nil
	}
	return claims
}

func extractVerifiedEmail(tokenData string) string {
	decoded := decodeToken(tokenData)
	inner := tokenObject(decoded)
	if inner == nil {
		return ""
	}
	idToken := firstString(inner["id_token"], getString(decoded, "id_token"))
	claims := decodeJWTClaims(idToken)
	if claims == nil {
		return ""
	}
	issuer := getString(claims, "iss")
	verified, ok := claims["email_verified"].(bool)
	if !oneOf(issuer, "accounts.google.com", "https://accounts.google.com") || !ok || !verified {
		return ""
	}
	return normalizeEmail(getString(claims, "email"))
}

func oauthClientID(decoded map[string]any) string {
	inner := tokenObject(decoded)
	idToken := firstString(inner["id_token"], getString(decoded, "id_token"))
	claims := decodeJWTClaims(idToken)
	aud := claims["aud"]
	switch value := aud.(type) {
	case string:
		if _, ok := oauthClientSecrets[value]; ok {
			return value
		}
	case []any:
		for _, raw := range value {
			if item, ok := raw.(string); ok {
				if _, found := oauthClientSecrets[item]; found {
					return item
				}
			}
		}
	}
	return defaultOAuthClientID
}

func tokenExpiry(inner map[string]any) (time.Time, bool) {
	if expiry := getString(inner, "expiry"); expiry != "" {
		parsed, err := parseUTC(expiry)
		return parsed, err == nil
	}
	if millis, ok := numberInt(inner["expiry_date"]); ok && millis > 0 {
		return time.UnixMilli(millis).UTC(), true
	}
	return time.Time{}, false
}

func (h *HTTPService) accessToken(ctx context.Context, tokenData string) (string, error) {
	decoded := decodeToken(tokenData)
	inner := tokenObject(decoded)
	if inner == nil {
		return "", fmt.Errorf("refresh token is unavailable")
	}
	if access := getString(inner, "access_token"); access != "" {
		if expiry, ok := tokenExpiry(inner); ok && time.Until(expiry) > 60*time.Second {
			return access, nil
		}
	}
	refresh := getString(inner, "refresh_token")
	if refresh == "" {
		return "", fmt.Errorf("refresh token is unavailable")
	}
	clientID := oauthClientID(decoded)
	values := url.Values{"client_id": {clientID}, "client_secret": {oauthClientSecrets[clientID]}, "refresh_token": {refresh}, "grant_type": {"refresh_token"}}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, h.oauthURL, strings.NewReader(values.Encode()))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("User-Agent", "Mozilla/5.0")
	requestCtx, cancel := context.WithTimeout(request.Context(), 15*time.Second)
	defer cancel()
	response, err := h.do(request.WithContext(requestCtx))
	if err != nil {
		return "", fmt.Errorf("OAuth refresh failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("OAuth refresh failed (HTTP %d)", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 4*1024*1024+1))
	if err != nil {
		return "", fmt.Errorf("OAuth refresh failed: %w", err)
	}
	if len(data) > 4*1024*1024 {
		return "", fmt.Errorf("OAuth refresh failed: response too large")
	}
	var result map[string]any
	if json.Unmarshal(data, &result) != nil {
		return "", fmt.Errorf("OAuth refresh failed: invalid response")
	}
	access := getString(result, "access_token")
	if access == "" {
		return "", fmt.Errorf("OAuth refresh returned no access token")
	}
	return access, nil
}

func (h *HTTPService) cloudPost(ctx context.Context, access, method string, body any) (map[string]any, error) {
	result, status, err := h.jsonRequest(ctx, http.MethodPost, h.cloudAPI+method, map[string]string{"Authorization": "Bearer " + access, "Content-Type": "application/json", "User-Agent": "antigravity"}, body, 15*time.Second, 4*1024*1024)
	if err != nil {
		if status != 0 {
			return nil, fmt.Errorf("%s failed (HTTP %d)", method, status)
		}
		return nil, fmt.Errorf("%s failed: %w", method, err)
	}
	return result, nil
}

func (h *HTTPService) userInfo(ctx context.Context, access string) map[string]any {
	result, _, err := h.jsonRequest(ctx, http.MethodGet, h.userInfoURL, map[string]string{"Authorization": "Bearer " + access}, nil, 5*time.Second, 4*1024*1024)
	if err != nil {
		return nil
	}
	return result
}
