# esm.sh

The [esm.sh](https://esm.sh) CLI/API for serving hot applications.

## Using the CLI tool

The CLI tool is used to run a hot application in current directory.

```bash
npx esm.sh -w
```

> The `-w` option is for watching the file changes to enable HMR.

## Using the API

The esm.sh API uses standard web APIs to serve hot applications.

```ts
export interface ServeOptions {
  /** The root path, default to current working directory. */
  root?: string;
  /** The fallback route, default is `index.html`. */
  fallback?: string;
  /** Wtaching file changes for HMR, default is `false` */
  watch?: boolean;
}

export function serveHost(
 options?: ServeOptions,
): (req: Request) => Promise<Response>;
```

For Node.js runtime, you need `@hono/server` to listen to the requests.

```js
import { serve } from "@hono/server";
import { serveHot } from "esm.sh";

serve({ port: 3000, fetch: serveHot() });
```

For Deno runtime, you can use `serveHot` directly.

```js
import { serveHot } from "https://esm.sh";

Deno.server(serveHot());
```

For Bun runtime:

```js
import { serveHot } from "esm.sh";

Bun.serve({ port: 3000, fetch: serveHot() });
```
