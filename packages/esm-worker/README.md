# esm-worker

A Cloudflare workers that handles all requests to the esm.sh origin server at
the edge.

- Cache everything by checking the `Cache-Control` header from the origin server
- Store modules in KV
- Store assets in R2

## Getting Started

```bash
npm install esm-worker
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
ESM_SERVER_ORIGIN = "https://esm.build"
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

export default worker((req, ctx) => {
  if (ctx.url.pathname === "/") {
    // custom the homepage
    return new Response("<h1>Welcome to use esm.sh!</h1>", {
      headers: { "content-type": "text/html" },
    });
  }
});
```
