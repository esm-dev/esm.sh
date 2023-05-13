# esm-worker

A [Cloudflare worker](https://www.cloudflare.com/products/workers) that handles
all requests of esm.sh at the edge(earth).

- [Cache](https://developers.cloudflare.com/workers/runtime-apis/cache/)
  everything at the edge
- Store ES6 modules in
  [KV](https://developers.cloudflare.com/workers/runtime-apis/kv)
- Store NPM/GH assets in
  [R2](https://developers.cloudflare.com/r2/api/workers/workers-api-reference)

## Getting Started

```bash
npm install esm-worker@0.120
```

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
ESM_SERVER_ORIGIN = "https://esm.sh"
NPM_REGISTRY = "https://registry.npmjs.org/"
# your other vars...

[[r2_buckets]]
binding = "R2"
bucket_name = "YOUR_BUCKET_NAME"
preview_bucket_name = "YOUR_PREVIEW_BUCKET_NAME"
```

Optional configurations in secrets:

- If you are using a private npm registry, you need to add the following
  configuration:
  ```bash
  wrangler secret put NPM_TOKEN
  ```
- If you are using a self-hosting esm.sh server with `authSecret` option, you need to
  add the following configuration:
  ```bash
  wrangler secret put ESM_SERVER_AUTH_TOKEN
  ```

Create a `.dev.vars` file for local development in the root directory of your
project:

```toml
WORKER_ENV = "development"
```

Then, wrap your worker with the `esm-worker` package:

```typescript
import worker from "esm-worker";

// extend the `Env` interface
declare global {
  interface Env {
    // your other vars in `wrangler.toml` ...
  }
}

export default worker((req, ctx) => {
  const { env, url } = ctx;

  // your routes override esm.sh routes
  if (url.pathname === "/") {
    // using the KV storage
    await env.KV.put("key", "value");
    const value = await env.KV.get("key");

    // using the R2 storage
    await env.R2.put("key", "value");
    const r2obj = await env.R2.get("key");

    if (env.WORKER_ENV === "development") {
      // local development
      // your code ...
    }

    // a custom homepage
    return new Response("<h1>Welcome to use esm.sh!</h1>", {
      headers: { "content-type": "text/html" },
    });

    // using cache
    return ctx.withCache(() =>
      new Response("Boom!", {
        headers: { "cache-control": "public; max-age=600" },
      })
    );
  }

});
```
