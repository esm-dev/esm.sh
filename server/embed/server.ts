import type {
  ConnInfo,
  ServeInit,
} from "https://deno.land/std@0.188.0/http/server.ts";
import {
  serve as stdServe,
} from "https://deno.land/std@0.188.0/http/server.ts";
import { dirname, join } from "https://deno.land/std@0.188.0/path/mod.ts";
import { ensureDir } from "https://deno.land/std@0.188.0/fs/ensure_dir.ts";
import type {
  Context,
  HttpMetadata,
  WorkerStorage,
} from "https://esm.sh/esm-worker@0.122.11";
import { withESMWorker } from "https://esm.sh/esm-worker@0.122.11";

type Handler = (
  request: Request,
  context: Context & ConnInfo,
) => Response | void | Promise<Response | void>;

const envKeys: (keyof Env)[] = [
  "ESM_SERVER_ORIGIN",
  "ESM_SERVER_AUTH_TOKEN",
  "NPM_REGISTRY",
  "NPM_TOKEN",
];

let env: Env = {};

export async function serve(handler: Handler, options?: ServeInit) {
  const worker = withESMWorker((req, _env, ctx) => {
    return handler?.(req, ctx as Context & ConnInfo);
  });
  if (!Reflect.has(env, "R2")) {
    Reflect.set(env, "R2", new FileStorage());
  }
  return await stdServe((req, connInfo) => {
    const context = {
      connInfo,
      waitUntil: () => {},
    };
    return worker.fetch(req, env, context);
  }, options);
}

type InitEnv = {
  storage?: WorkerStorage | null;
} & Env;

export function init(initEnv: InitEnv = {}) {
  const { storage, ...rest } = initEnv;
  if (storage) {
    Reflect.set(env, "R2", storage);
  }
  envKeys.forEach((key) => {
    if (!env[key]) {
      const value = Deno.env.get(key);
      if (value) {
        Reflect.set(env, key, value);
      }
    }
  });
  env = rest;
}

export class FileStorage implements WorkerStorage {
  #rootDir?: string;

  constructor(rootDir?: string) {
    this.#rootDir = rootDir;
  }

  get rootDir(): Promise<string> {
    if (this.#rootDir) {
      return Promise.resolve(this.#rootDir);
    }
    return getGlobalCacheDir().then((dir) => {
      this.#rootDir = dir;
      return dir;
    });
  }

  async get(key: string): Promise<
    {
      body: ReadableStream<Uint8Array>;
      httpMetadata?: HttpMetadata;
    } | null
  > {
    const filepath = join(await this.rootDir, await hashKey(key));
    try {
      const file = await Deno.open(filepath);
      let httpMetadata: HttpMetadata | undefined;
      try {
        const data = await Deno.readTextFile(filepath + ".metadata");
        try {
          httpMetadata = JSON.parse(data);
        } catch (_) {
          // ignore error
        }
      } catch (err) {
        if (!(err instanceof Deno.errors.NotFound)) {
          throw err;
        }
      }
      return {
        body: file.readable,
        httpMetadata,
      };
    } catch (err) {
      if (err instanceof Deno.errors.NotFound) {
        return null;
      }
      throw err;
    }
  }

  async put(
    key: string,
    value: ArrayBufferLike | ArrayBuffer | ReadableStream,
    options?: { httpMetadata?: HttpMetadata },
  ): Promise<void> {
    const filepath = join(await this.rootDir, await hashKey(key));
    await ensureDir(dirname(filepath));
    if (value instanceof ReadableStream) {
      const file = await Deno.open(filepath, { create: true, write: true });
      await value.pipeTo(file.writable);
    } else {
      await Deno.writeFile(filepath, new Uint8Array(value));
    }
    try {
      if (options?.httpMetadata) {
        await Deno.writeTextFile(
          filepath + ".metadata",
          JSON.stringify(options.httpMetadata),
        );
      }
    } catch (_) {
      // ignore error
    }
  }
}

async function getGlobalCacheDir() {
  const command = new Deno.Command(Deno.execPath(), {
    args: ["info", "--json"],
  });
  const { code, stdout } = await command.output();
  if (code !== 0) {
    throw new Error("Failed to run `deno info --json`");
  }
  const info = JSON.parse(new TextDecoder().decode(stdout));
  return join(info.denoDir, "esm.sh");
}

async function hashKey(key: string): Promise<string> {
  const buffer = await crypto.subtle.digest(
    "SHA-256",
    new TextEncoder().encode(key),
  );
  // return hex string
  return [...new Uint8Array(buffer)].map((b) => b.toString(16).padStart(2, "0"))
    .join("");
}

if (import.meta.main) {
  const { parse } = await import("https://deno.land/std@0.188.0/flags/mod.ts");
  const flags = parse(Deno.args);
  const port = flags.port || 8787;
  serve((req) => {
    const url = new URL(req.url);
    if (url.pathname === "/") {
      return new Response(
        `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>ESM&gt;CDN</title>
  <style>
    * {
      padding: 0;
      margin: 0;
    }
    body {
      display: flex;
      align-items: center;
      justify-content: center;
      height: 100vh;
      overflow: hidden;
    }
    h1 {
      font-size: 32px;
      line-height: 1.5;
      font-family: Inter, sans-serif;
      text-align: center;
    }
    p {
      font-size: 18px;
      text-align: center;
      line-height: 1.2;
      color: #888;
    }
    pre {
      margin-top: 23px;
      background-color: #f3f3f3;
      padding: 16px 20px;
      border-radius: 8px;
      font-size: 14px;
    }
  </style>
</head>
<body>
  <div id="app">
    <h1>Welcome to use esm.sh!</h1>
    <p>This is local version of esm.sh running on Deno.</p>
    <pre><code>import React from "http://localhost:${port}/react"</code></pre>
  </div>
</body>
</html>
`,
        {
          headers: { "Content-Type": "text/html; charset=utf-8" },
        },
      );
    }
  }, {
    port,
  });
}

export default serve;
