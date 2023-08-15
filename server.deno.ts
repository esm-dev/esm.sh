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
} from "https://esm.sh/esm-worker@0.131.0";
import { withESMWorker } from "https://esm.sh/esm-worker@0.131.0";

type Handler = (
  request: Request,
  context: Context & ConnInfo,
) => Response | void | Promise<Response | void>;

const isDenoDeploy = Deno.env.has("DENO_DEPLOYMENT_ID");
const envKeys: (keyof Env)[] = [
  "ESM_ORIGIN",
  "ESM_TOKEN",
  "NPM_REGISTRY",
  "NPM_TOKEN",
];

let env: Env = {};

export async function serve(handler?: Handler, options?: ServeInit) {
  const worker = withESMWorker((req, _env, ctx) => {
    return handler?.(req, ctx as Context & ConnInfo);
  });
  if (!Reflect.has(env, "R2") && !isDenoDeploy) {
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
      const data = await Deno.readTextFile(filepath + ".metadata.json");
      const metadata = JSON.parse(data);
      return {
        body: file.readable,
        httpMetadata: metadata.httpMetadata,
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
      const metadata = {
        key,
        time: Date.now(),
        httpMetadata: options?.httpMetadata,
      };
      await Deno.writeTextFile(
        filepath + ".metadata.json",
        JSON.stringify(metadata, undefined, 2),
      );
    } catch (err) {
      await Deno.remove(filepath);
      throw err;
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

if (import.meta.main && !isDenoDeploy) {
  const { parse } = await import("https://deno.land/std@0.188.0/flags/mod.ts");
  const flags = parse(Deno.args);
  if (flags.help || flags.h) {
    console.log(
      "%cWelcome to esm.sh!",
      "font-weight:bold;color:#007bff;",
    );
    console.log(
      "%cThis is local version of esm.sh running on %cDeno 🦕%c.",
      "color:gray;","color: green", "color:gray;"
    );
    console.log("");
    console.log("Usage:");
    console.log("  deno run -A -r https://esm.sh/server");
    console.log("");
    console.log("Options:");
    console.log("  --port <port>    Port to listen on. Default is 8787.");
    console.log("  --help, -h       Print this help message.");
    console.log("");
    console.log("ENVIRONMENT VARIABLES:");
    console.log("  ESM_ORIGIN    The origin of esm.sh server.");
    console.log("  ESM_TOKEN     The token of esm.sh server.");
    console.log("  NPM_REGISTRY  The npm registry, Default is 'https://registry.npmjs.org/'.");
    console.log("  NPM_TOKEN     The npm token.");
    Deno.exit(0);
  }
  init();
  const port = flags.port || 8787;
  serve((_req, { url }) => {
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
      flex-direction: column;
      height: 100vh;
      overflow: hidden;
    }
    h1 {
      margin-bottom: 8px;
      font-size: 40px;
      line-height: 1;
      font-family: Inter,sans-serif;
      text-align: center;
    }
    p {
      font-size: 18px;
      line-height: 1;
      color: #888;
      text-align: center:
    }
    pre {
      margin-top: 24px;
      background-color: #f3f3f3;
      padding: 18px;
      border-radius: 8px;
      font-size: 14px;
      line-height: 1;
    }
    pre .keyword {
      color: #888;
    }
    pre a {
      color: #58a65c;
      text-decoration: underline;
    }
    pre a:hover {
      opacity: 0.8;
    }
  </style>
</head>
<body>
  <h1>Welcome to esm.sh!</h1>
  <p>This is local version of esm.sh running on Deno 🦕.</p>
  <pre><code><span class="keyword">import</span> React <span class="keyword">from</span> "<a href="${url.origin}/react" class="url">${url.origin}/react</a>"</code></pre>
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
    onListen: () => {
      console.log(
        "%cWelcome to esm.sh!",
        "font-weight:bold;color:#007bff;",
      );
      console.log(
        "%cThis is local version of esm.sh running on %cDeno 🦕%c.",
        "color:gray;","color: green", "color:gray;"
      );
      console.log(
        `Homepage: %chttp://localhost:${port}`,
        "color:gray;",
        "",
      );
      console.log(
        `Module: %chttp://localhost:${port}/react`,
        "color:gray;",
        "",
      );
    },
  });
}

export default serve;
