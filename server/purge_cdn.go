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

// When `cloudflareZoneId`/`cloudflareApiToken` are configured a purge also
// drops the matching entries from the Cloudflare edge cache, so clients do not
// keep serving the stale build until its TTLs expire. Cloudflare purges by
// exact URL (max 30 per request) on non-Enterprise plans, so the public URLs
// are derived from the removed storage keys.
const cloudflarePurgeBatchSize = 30

var cloudflareAPIBaseURL = "https://api.cloudflare.com"

// purgeHTTPClient is a short-timeout client for the OAuth and CDN APIs, kept
// separate from the module-fetch client.
var purgeHTTPClient = &http.Client{Timeout: 15 * time.Second}

// cdnPurgeURLs maps the storage keys removed by a purge back to the public URLs
// a CDN may have cached. Keys whose public form cannot be reconstructed (build
// args are hashed into `x-<sha1>` path segments) are skipped.
func cdnPurgeURLs(origin string, pkgId string, keys []string) []string {
	seen := map[string]bool{}
	urls := []string{}
	add := func(u string) {
		if !seen[u] {
			seen[u] = true
			urls = append(urls, u)
		}
	}
	add(origin + "/" + pkgId)
	for _, key := range keys {
		var pathname string
		switch {
		case strings.HasPrefix(key, "modules/"):
			pathname = key[len("modules/"):]
		case strings.HasPrefix(key, "types/"):
			pathname = key[len("types/"):]
		default:
			continue
		}
		if cdnPathIsPurgeable(pathname) {
			add(origin + "/" + pathname)
		}
	}
	return urls
}

// cdnPathIsPurgeable reports whether a storage pathname has an unambiguous
// public URL. Paths with hashed build-args segments or the `*` (external all)
// marker cannot be mapped back and are skipped.
func cdnPathIsPurgeable(pathname string) bool {
	if strings.HasPrefix(pathname, "transform/") || strings.HasPrefix(pathname, "x/") {
		return false
	}
	for _, seg := range strings.Split(pathname, "/") {
		if strings.HasPrefix(seg, "X-") || strings.HasPrefix(seg, "x-") || seg == "ea" {
			return false
		}
	}
	return true
}

// purgeCloudflareCache asks Cloudflare to evict the given URLs from its edge
// cache. Failures are logged but never fail the purge itself.
func purgeCloudflareCache(logger *log.Logger, urls []string) {
	if config.CloudflareZoneID == "" || config.CloudflareAPIToken == "" || len(urls) == 0 {
		return
	}
	endpoint := cloudflareAPIBaseURL + "/client/v4/zones/" + config.CloudflareZoneID + "/purge_cache"
	for start := 0; start < len(urls); start += cloudflarePurgeBatchSize {
		end := min(start+cloudflarePurgeBatchSize, len(urls))
		body, _ := json.Marshal(map[string][]string{"files": urls[start:end]})
		req, _ := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+config.CloudflareAPIToken)
		res, err := purgeHTTPClient.Do(req)
		if err != nil {
			logger.Errorf("cloudflare purge: %v", err)
			return
		}
		resBody, _ := io.ReadAll(io.LimitReader(res.Body, MB))
		res.Body.Close()
		if res.StatusCode >= 400 {
			logger.Errorf("cloudflare purge: %s: %s", res.Status, strings.TrimSpace(string(resBody)))
			return
		}
	}
	logger.Debugf("cloudflare purge: evicted %d URLs", len(urls))
}
