#!/usr/bin/env node

import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { serve } from "../vendor/hono-server@1.3.3.mjs";
import init from "../vendor/html-rewriter@0.4.1.mjs";
import { serveHot } from "../src/index.mjs";

if (process.argv.includes("--help") || process.argv.includes("-h")) {
  console.log(`
Usage: npx esm.sh [options] [root]

Options:
  --help, -h      Show help message
  --host          Host to listen on (default: "localhost")
  --port, -p      Port number to listen on (default: 3000)
`);
  process.exit(0);
}

const args = {
  port: 3000,
};

process.argv.slice(2).forEach((arg) => {
  if (!arg.startsWith("-")) {
    if (existsSync(arg)) {
      args.root = arg;
    }
    return;
  }
  const [key, value] = arg.split("=");
  if ((key === "--port" || key === "-p") && value) {
    args.port = parseInt(value);
    if (isNaN(args.port) && args.port <= 0) {
      throw new Error("Invalid port number");
    }
  } else if (key === "--host" && value) {
    args.host = value;
  }
});

// init HTMLRewriter wasm module
await init();

const dotEnvPath = join(args.root ?? process.cwd(), ".env");
if (existsSync(dotEnvPath)) {
  const env = Object.fromEntries(
    readFileSync(dotEnvPath, "utf-8")
      .split("\n").map((line) => line.trim())
      .filter((line) => line && !line.startsWith("#"))
      .map((line) => {
        const kv = line.split("#")[0].trim();
        const idx = kv.indexOf("=");
        if (idx < 0) {
          return [kv, ""];
        }
        return [
          kv.slice(0, idx).trimEnd(),
          kv.slice(idx + 1).trimStart(),
        ];
      }),
  );
  Object.assign(process.env, env);
  console.log("Found project '.env'");
}

serve(
  { ...args, fetch: serveHot(args) },
  (info) => {
    console.log(`Listening on http://${args.host ?? "localhost"}:${info.port}`);
  },
);
