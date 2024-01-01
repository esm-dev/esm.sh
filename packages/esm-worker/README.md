# esm-worker

A [Cloudflare worker](https://www.cloudflare.com/products/workers) handles all
requests of esm.sh at the edge(earth).

- [Cache](https://developers.cloudflare.com/workers/runtime-apis/cache/)
  everything at the edge
- Store ES6 modules in
  [KV](https://developers.cloudflare.com/workers/runtime-apis/kv)
- Store NPM/GH assets in
  [R2](https://developers.cloudflare.com/r2/api/workers/workers-api-reference)

## Installation

```bash
npm install esm-worker@^0.135.0
```

## Configuration

You need to add following configuration to your `wrangler.toml`:

```toml
kv_namespaces = [
  {
    binding = "KV",
    id = "YOUR_KV_ID",
    preview_id = "YOUR_PREVIEW_KV_ID"
  },
  # your other namespaces...
]

[vars]
ESM_ORIGIN = "https://esm.sh" # change to your self-hosting esm.sh server if needed
NPM_REGISTRY = "https://registry.npmjs.org/" # change to your private npm registry if needed
# your other vars...

[[r2_buckets]]
binding = "R2"
bucket_name = "YOUR_BUCKET_NAME"
preview_bucket_name = "YOUR_PREVIEW_BUCKET_NAME"
```

Other optional configurations in secrets:

- If you are using a self-hosting esm.sh server with `authSecret` option, you
  need to add the following configuration:
  ```bash
  wrangler secret put ESM_TOKEN
  ```
- If you are using a private npm registry, you need to add the following
  configuration:
  ```bash
  wrangler secret put NPM_TOKEN
  ```

## Usage

Wrap your Cloudflare worker with the `esm-worker` package:

```typescript
import { withESMWorker } from "esm-worker";

// extend the `Env` interface
declare global {
  interface Env {
    // your other vars in `wrangler.toml` ...
  }
}

export default withESMWorker((req, env, ctx) => {
  const { url } = ctx;

  // your routes override esm.sh routes
  if (url.pathname === "/") {
    // using a custom homepage
    return new Response("<h1>Welcome to esm.sh!</h1>", {
      headers: { "Content-Type": "text/html" },
    });

    // using cache
    return ctx.withCache(() =>
      new Response("Boom!", {
        headers: { "Cache-Control": "public; max-age=3600" },
      })
    );
  }
});
```

## Deploy to Cloudflare Edge

```bash
wrangler deploy
```
