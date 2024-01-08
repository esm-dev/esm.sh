# esm.sh

The CLI/API for serving `esm.sh/hot` applications.

## What is esm.sh/hot ?

[TODO]

## Using the CLI tool

The CLI starts a server using the current working directory as the root path of the esm.sh/hot application.

```bash
$ npx esm.sh
Listening on http://localhost:3000
```

you can specify the root path as well:

```bash
$ npx esm.sh my-app
```

## Using the API

The esm.sh API uses standard web APIs to serve requests of a hot application.

```ts
export interface ServeOptions {
  /** The root path, default to current working directory. */
  root?: string;
}

export function serveHost(
 options?: ServeOptions,
): (req: Request) => Promise<Response>;
```

For Node.js runtime, you need `@hono/server` to listen to the http requests.

```js
import { serve } from "@hono/server";
import { serveHot } from "esm.sh";
import injectCompatLayer from "esm.sh/compat";

await injectCompatLayer();
serve({ port: 3000, fetch: serveHot() });
```

For Deno runtime, you can use `serveHot` directly.

```js
import { serveHot } from "https://esm.sh/hot/server";

Deno.server(serveHot());
```

For Bun runtime:

```js
import { serveHot } from "esm.sh";

Bun.serve({ port: 3000, fetch: serveHot() });
```
