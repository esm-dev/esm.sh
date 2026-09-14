package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ije/gox/log"
)

var cloudflareAPIBaseURL = "https://api.cloudflare.com"

// purgeHTTPClient is a short-timeout client for the OAuth and CDN APIs, kept
// separate from the module-fetch client.
var purgeHTTPClient = &http.Client{Timeout: 15 * time.Second}

// purgeCloudflareCache evicts all URL variants under the package's normal and
// external-all prefixes, even when its origin artifacts have already been removed.
// Failures are logged but never fail the origin purge.
func purgeCloudflareCache(logger *log.Logger, origin string, pkgId string) {
	if config.PurgeAPI.CloudflareZoneID == "" || config.PurgeAPI.CloudflareAPIToken == "" || origin == "" {
		return
	}
	// Cloudflare prefixes include the hostname but omit the scheme.
	origin = strings.TrimPrefix(strings.TrimPrefix(origin, "https://"), "http://")
	body, _ := json.Marshal(map[string][]string{"prefixes": {origin + "/" + pkgId, origin + "/*" + pkgId}})
	endpoint := cloudflareAPIBaseURL + "/client/v4/zones/" + config.PurgeAPI.CloudflareZoneID + "/purge_cache"
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		logger.Errorf("cloudflare purge: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.PurgeAPI.CloudflareAPIToken)
	res, err := purgeHTTPClient.Do(req)
	if err != nil {
		logger.Errorf("cloudflare purge: %v", err)
		return
	}
	resBody, err := io.ReadAll(io.LimitReader(res.Body, MB))
	res.Body.Close()
	if err != nil {
		logger.Errorf("cloudflare purge: %v", err)
		return
	}
	var result struct {
		Success bool `json:"success"`
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 || json.Unmarshal(resBody, &result) != nil || !result.Success {
		logger.Errorf("cloudflare purge: %s: %s", res.Status, strings.TrimSpace(string(resBody)))
		return
	}
	logger.Debugf("cloudflare purge: evicted %s", pkgId)
}
