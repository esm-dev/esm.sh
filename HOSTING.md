# Self-Hosting

[esm.sh](https://esm.sh) provides a global fast CDN publicly which is powered by [Cloudflare](https://cloudflare.com).
You can also host esm.sh service by yourself. To do this, please follow the instructions below.

## Clone the Source Code

```bash
git clone https://github.com/esm-dev/esm.sh
cd esm.sh
```

## Configuration

To configure the server, create a `config.json` file then pass it to the server bootstrap command. For example:

```jsonc
// config.json
{
  "port": 8080,
  "npmRegistry": "https://registry.npmjs.org/",
  "npmToken": "******"
}
```

You can find all the server options in [config.example.jsonc](./config.example.jsonc).

### Cache Purge

Cache purging is enabled by default. Open `/purge` on your server to refresh a package's cache:

- An exact version removes its builds, source maps, type declarations, build metadata, and local npm store copy.
- A bare name, dist-tag, range, date, or GitHub branch only refreshes version resolution. Existing builds are reused if the version has not changed.

Every `POST /purge` requires a single-use proof-of-work challenge from `GET /pow/challenge?scope=purge`, valid for two minutes. The page solves it automatically; scripts can follow the [API example](./README.md#purge-cache) using your server's origin. Requests are limited to five per minute per client IP, or per GitHub account when login is enabled.

Client IPs come from the connection by default. Behind a reverse proxy, set `trustedProxies` to its CIDRs, for example `["127.0.0.1/32", "::1/128"]` for a local proxy. Forwarded addresses are checked from right to left through those proxies. Configure the proxy to append the actual client address to `X-Forwarded-For`, or overwrite `X-Real-IP` when it does not send `X-Forwarded-For`. Only list networks you control or trust.

Configure purging under `purgeAPI` in `config.json`:

```json
{
  "purgeAPI": {
    "enable": true,
    "githubClientId": "",
    "githubClientSecret": "",
    "cloudflareZoneId": "",
    "cloudflareApiToken": ""
  }
}
```

Set `purgeAPI.enable` to `false` or `PURGE_CACHE=false` to disable purging (`POST /purge` returns 403). Non-empty credentials in `purgeAPI` take precedence over environment variables.

#### GitHub Login

To require login, [create a GitHub OAuth app](https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/creating-an-oauth-app) with your server's public origin as its homepage and `https://cdn.example.com/purge/callback` as its authorization callback URL. Replace `cdn.example.com` with your hostname.

Set both `purgeAPI.githubClientId` and `purgeAPI.githubClientSecret`, or `PURGE_GITHUB_CLIENT_ID` and `PURGE_GITHUB_CLIENT_SECRET`. Set `cdnOrigin` or `CDN_ORIGIN` to your public origin, such as `https://cdn.example.com`, so OAuth redirects use the correct origin behind a proxy.

The purge page then requires GitHub sign-in. Any signed-in GitHub user can purge packages; package ownership is not checked. API clients need the signed session cookie as well as a solved proof-of-work challenge.

#### Cloudflare Cache Purging

Set both `purgeAPI.cloudflareZoneId` and `purgeAPI.cloudflareApiToken`, or `PURGE_CLOUDFLARE_ZONE_ID` and `PURGE_CLOUDFLARE_API_TOKEN`. Use an API token with [Cache Purge permission](https://developers.cloudflare.com/api/resources/cache/methods/purge/) for the target zone, and set `cdnOrigin` or `CDN_ORIGIN` to the public CDN origin.

Exact-version purges also [purge Cloudflare by prefix](https://developers.cloudflare.com/cache/how-to/purge-cache/purge_by_prefix/) for the package's normal and external-all paths, including subpaths, build arguments and query-string variants. Prefix purging is available on all Cloudflare plans. Floating specifiers only refresh resolution at the origin. Cloudflare request failures are logged without failing the origin purge; repeating the purge retries the same prefixes even after origin artifacts have been removed.

Add the [cache bypass rule](#5-bypass-cache-for-purge-endpoints) below to keep challenges and login responses out of the CDN cache.

## Run the Server Locally

You will need [Go](https://golang.org/dl) 1.25+ to compile and run the server.

```bash
go run server/esmd/main.go --config=config.json
```

Then you can import `React` from <http://localhost:8080/react>.

## Deploy the Server to a Single Machine

You can deploy the server to a single machine with the [deploy.sh](./scripts/deploy.sh) script.

```bash
# first time to deploy
./scripts/deploy.sh --init
# update the server
./scripts/deploy.sh
```

Recommended hosting requirements:

- Linux system with systemd installed
- 4x CPU cores or more
- 8GB RAM or more
- 100GB disk space or more

## Deploy with Docker

[![Docker Image](https://img.shields.io/github/v/tag/esm-dev/esm.sh?label=Docker&display_name=tag&sort=semver&style=flat&colorA=232323&colorB=232323&logo=docker&logoColor=eeeeee)](https://github.com/esm-dev/esm.sh/pkgs/container/esm.sh)

esm.sh provides a Docker image for fast deployment. You can pull the container image from <https://ghcr.io/esm-dev/esm.sh>.

```bash
docker pull ghcr.io/esm-dev/esm.sh        # latest stable version
docker pull ghcr.io/esm-dev/esm.sh:v137   # specified stable version
docker pull ghcr.io/esm-dev/esm.sh:dev    # latest dev version
```

Run the container:

```bash
docker run -e CDN_ORIGIN=https://cdn.example.com -p 80:80 ghcr.io/esm-dev/esm.sh:latest
```

Available environment variables:

- `CDN_ORIGIN`: The public CDN origin, default is empty (use the request origin).
- `COMPRESS`: Compress http responses with gzip/brotli, default is `true`.
- `CUSTOM_LANDING_PAGE_ORIGIN`: The custom landing page origin, default is empty.
- `CUSTOM_LANDING_PAGE_ASSETS`: The custom landing page assets separated by comma(,), default is empty.
- `CORS_ALLOW_ORIGINS`: The CORS allow origins separated by comma(,), default is allow all origins.
- `LOG_LEVEL`: The log level, available values are ["debug", "info", "warn", "error"], default is "info".
- `ACCESS_LOG`: Enable access log, default is `false`.
- `MINIFY`: Minify the built JS/CSS files, default is `true`.
- `NPM_QUERY_CACHE_TTL`: The cache TTL for NPM query, default is 10 minutes.
- `NPM_REGISTRY`: The global NPM registry, default is "https://registry.npmjs.org/".
- `NPM_TOKEN`: The access token for the global NPM registry.
- `NPM_USER`: The access user for the global NPM registry.
- `NPM_PASSWORD`: The access password for the global NPM registry.
- `PURGE_CACHE`: Enable cache purging, default is `true`.
- `PURGE_GITHUB_CLIENT_ID`: The GitHub OAuth app client ID for [purge login](#github-login).
- `PURGE_GITHUB_CLIENT_SECRET`: The GitHub OAuth app client secret. Both GitHub settings are required to enable login.
- `PURGE_CLOUDFLARE_ZONE_ID`: The Cloudflare zone ID for [cache purging](#cloudflare-cache-purging).
- `PURGE_CLOUDFLARE_API_TOKEN`: The Cloudflare API token with Cache Purge permission. Both Cloudflare settings are required to enable edge purging.
- `SOURCEMAP`: Generate source map for built JS/CSS files, default is `true`.
- `STORAGE_TYPE`: The storage type, available values are ["fs", "s3"], default is "fs".
- `STORAGE_ENDPOINT`: The storage endpoint, default is "~/.esmd/storage".
- `STORAGE_REGION`: The region for S3 storage.
- `STORAGE_ACCESS_KEY_ID`: The access key for S3 storage.
- `STORAGE_SECRET_ACCESS_KEY`: The secret key for S3 storage.

You can also create your own Dockerfile with a `config.json` file:

```dockerfile
FROM ghcr.io/esm-dev/esm.sh:latest
ADD --chown=esm:esm ./config.json /etc/esmd/config.json
CMD ["esmd", "--config", "/etc/esmd/config.json"]
```

## Deploy with CloudFlare CDN

To deploy the server with CloudFlare CDN, create the following cache rules in the CloudFlare dashboard (see [link](https://developers.cloudflare.com/cache/how-to/cache-rules/create-dashboard/)). Set rules 1–4 to **"Eligible for cache"**, and the purge endpoint rule to **"Bypass cache"**:

#### 1. Cache Static Content

```ruby
(http.request.uri.path.extension in {"node" "wasm" "less" "sass" "scss" "stylus" "styl" "json" "jsonc" "csv" "xml" "plist" "tmLanguage" "tmTheme" "yml" "yaml" "txt" "glsl" "frag" "vert" "wgsl" "md" "mdx" "markdown" "html" "htm" "svg" "png" "jpg" "jpeg" "webp" "gif" "ico" "eot" "ttf" "otf" "woff" "woff2" "m4a" "mp3" "m3a" "ogg" "oga" "wav" "weba" "gz" "tgz" "css" "map"}) or
(http.request.uri.path.extension eq "mjs" and starts_with(http.request.uri.path, "/node/")) or
(ends_with(http.request.uri.path, ".d.ts")) or
(ends_with(http.request.uri.path, ".d.mts")) or
(ends_with(http.request.uri.path, ".d.cts"))
```

Ensure "Ignore query string" option is enabled in the "cache key" settings.

#### 2. Cache `?target=*`

```ruby
(http.request.uri contains "target=es2015") or
(http.request.uri contains "target=es2016") or
(http.request.uri contains "target=es2017") or
(http.request.uri contains "target=es2018") or
(http.request.uri contains "target=es2019") or
(http.request.uri contains "target=es2020") or
(http.request.uri contains "target=es2021") or
(http.request.uri contains "target=es2022") or
(http.request.uri contains "target=es2023")or
(http.request.uri contains "target=es2024") or
(http.request.uri contains "target=esnext") or
(http.request.uri contains "target=denonext") or
(http.request.uri contains "target=deno") or
(http.request.uri contains "target=node")
```

#### 3. Cache `/(target)/`

```ruby
(http.request.uri.path contains "/es2015/" and http.request.uri.path.extension in {"mjs" "map" "css"}) or
(http.request.uri.path contains "/es2016/" and http.request.uri.path.extension in {"mjs" "map" "css"}) or
(http.request.uri.path contains "/es2017/" and http.request.uri.path.extension in {"mjs" "map" "css"}) or
(http.request.uri.path contains "/es2018/" and http.request.uri.path.extension in {"mjs" "map" "css"}) or
(http.request.uri.path contains "/es2019/" and http.request.uri.path.extension in {"mjs" "map" "css"}) or
(http.request.uri.path contains "/es2020/" and http.request.uri.path.extension in {"mjs" "map" "css"}) or
(http.request.uri.path contains "/es2021/" and http.request.uri.path.extension in {"mjs" "map" "css"}) or
(http.request.uri.path contains "/es2022/" and http.request.uri.path.extension in {"mjs" "map" "css"}) or
(http.request.uri.path contains "/es2023/" and http.request.uri.path.extension in {"mjs" "map" "css"}) or
(http.request.uri.path contains "/es2024/" and http.request.uri.path.extension in {"mjs" "map" "css"}) or
(http.request.uri.path contains "/esnext/" and http.request.uri.path.extension in {"mjs" "map" "css"}) or
(http.request.uri.path contains "/denonext/" and http.request.uri.path.extension in {"mjs" "map" "css"}) or
(http.request.uri.path contains "/deno/" and http.request.uri.path.extension in {"mjs" "map" "css"}) or
(http.request.uri.path contains "/node/" and http.request.uri.path.extension in {"mjs" "map" "css"})
```

#### 4. Bypass Cache for Deno/Bun/Node

```ruby
(not starts_with(http.user_agent, "Deno/") and not starts_with(http.user_agent, "Bun/") and not starts_with(http.user_agent, "Node/") and not starts_with(http.user_agent, "Node.js/") and http.user_agent ne "undici")
```

> [!NOTE]
> Since Cloudflare does not respect the `Vary` header, we need to bypass the cache for Node/Deno/Bun runtime.

#### 5. Bypass Cache for Purge Endpoints

Set this rule to **"Bypass cache"** and place it after the cache eligibility rules, since the [last matching rule takes precedence](https://developers.cloudflare.com/cache/how-to/cache-rules/order/):

```ruby
(http.request.uri.path eq "/purge") or
(starts_with(http.request.uri.path, "/purge/")) or
(http.request.uri.path eq "/pow/challenge")
```
