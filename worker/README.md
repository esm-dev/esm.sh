# esm-worker

A [Cloudflare worker](https://www.cloudflare.com/products/workers) handles all requests of esm.sh at the edge(earth).

## Installation

```bash
npm install esm-worker@0.135
```

## Configuration

You need to add following configuration to your `wrangler.toml`:

```toml
[vars]
ESM_SERVER_ORIGIN = "https://esm.sh/" # change to your self-hosting esm.sh server
NPM_REGISTRY = "https://registry.npmjs.org/" # change to your private npm registry if needed
# your other vars...

[[r2_buckets]]
binding = "R2"
bucket_name = "YOUR_BUCKET_NAME"
preview_bucket_name = "YOUR_PREVIEW_BUCKET_NAME"
```

Other optional configurations in secrets:

- If you are using a self-hosting esm.sh server with `authSecret` option, you need to add the following configuration:
  ```bash
  wrangler secret put ESM_SERVER_TOKEN
  ```
- If you are using a private npm registry, you need to add the following configuration:
  ```bash
  wrangler secret put NPM_TOKEN
  ```

## Usage

Wraps your Cloudflare worker with the `withESMWorker` function:

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

  // use a custom homepage
  if (url.pathname === "/") {
    return new Response("<h1>Welcome to esm.sh!</h1>", {
      headers: { "Content-Type": "text/html" },
    });
  }

  // use the cache API
  if (url.pathname === "/boom") {
    return ctx.withCache(() =>
      new Response("Boom!", {
        headers: { "Cache-Control": "public; max-age=3600" },
      })
    );
  }

  // let esm-worker handles the rest
  return ctx.next();
});
```

## Deploy to Cloudflare Workers

```bash
wrangler deploy
```
