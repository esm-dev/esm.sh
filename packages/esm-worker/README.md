# esm-worker

A Cloudflare workers that handles all requests to the esm.sh origin server at
the edge.

- Cache everything by checking the `Cache-Control` header from the origin server
- Store modules in KV
- Store assets in R2

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
ESM_SERVER_ORIGIN = "https://YOUR_ORIGIN_ESM_HOSTNAME"
ESM_SERVER_AUTH_TOKEN = "" # optional
WORKER_ENV = "production"
# your other vars...

[[r2_buckets]]
binding = "R2"
bucket_name = "YOUR_BUCKET_NAME"
preview_bucket_name = "YOUR_PREVIEW_BUCKET_NAME"
```

create a `.dev.vars` file for local development in the root directory of your
project:

```toml
WORKER_ENV = "development"
```

Then, wrap your worker with the `esm-worker` package:

```js
import worker from "esm-worker";

// extend the `Env` interface
declare global {
  interface Env {
    // your other vars in `wrangler.toml`...
  }
}

export default worker((req, ctx) => {
  const { env, isDev, url } = ctx;
  if (url.pathname === "/") {
    // custom the homepage
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

  // using KV
  await env.KV.put("key", "value");
  const value = await env.KV.get("key");

  // using R2
  await env.R2.put("key", "value");
  const r2obj = await env.R2.get("key");

  if (isDev) {
    // local development
    // your code...
  }
});
```
