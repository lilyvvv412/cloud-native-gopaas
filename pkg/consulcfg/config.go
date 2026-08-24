package consulcfg

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// KV holds a simple configuration document stored in Consul KV.
type KV struct {
	RateLimitQPS int               `json:"rate_limit_qps"`
	FeatureFlags map[string]bool   `json:"feature_flags"`
	Extra        map[string]string `json:"extra"`
}

// LoadJSON fetches a JSON blob from Consul KV. Missing keys return defaults.
func LoadJSON(consulAddr, key string, defaults KV) KV {
	if consulAddr == "" {
		return defaults
	}
	u := url.URL{
		Scheme: "http",
		Host:   strings.TrimPrefix(strings.TrimPrefix(consulAddr, "http://"), "https://"),
		Path:   "/v1/kv/" + strings.TrimPrefix(key, "/"),
	}
	q := u.Query()
	q.Set("raw", "true")
	u.RawQuery = q.Encode()

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(u.String())
	if err != nil {
		log.Printf("consul config fetch skipped: %v", err)
		return defaults
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return defaults
	}
	if resp.StatusCode != http.StatusOK {
		log.Printf("consul config unexpected status %d", resp.StatusCode)
		return defaults
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return defaults
	}
	var out KV
	if err := json.Unmarshal(body, &out); err != nil {
		log.Printf("consul config decode failed: %v", err)
		return defaults
	}
	if out.RateLimitQPS == 0 {
		out.RateLimitQPS = defaults.RateLimitQPS
	}
	if out.FeatureFlags == nil {
		out.FeatureFlags = defaults.FeatureFlags
	}
	return out
}

// PutJSON is a helper used by demos/scripts to seed Consul KV.
func PutJSON(consulAddr, key string, value KV) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	u := fmt.Sprintf("http://%s/v1/kv/%s", strings.TrimPrefix(strings.TrimPrefix(consulAddr, "http://"), "https://"), strings.TrimPrefix(key, "/"))
	req, err := http.NewRequest(http.MethodPut, u, strings.NewReader(string(raw)))
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("consul put status %s", resp.Status)
	}
	return nil
}

// MustInt parses an int or returns fallback.
func MustInt(s string, fallback int) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return n
}
