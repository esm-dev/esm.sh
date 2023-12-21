# esm.sh

The CLI/API for serving `esm.sh/hot` applications.

## What is esm.sh/hot ?

[TODO]

## Using the CLI tool

To use the CLI tool, you need to install it globally on your
machine:

```bash
npm i -g esm.sh
```

or using `npx` to run it directly:

```bash
npx esm.sh
```

The CLI will start a esm.sh/hot server using the current working directory as the root path.

```bash
$ esm.sh
Listening on http://localhost:3000
```

or you can specify the root path:

```bash
$ esm.sh my-app
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
