#!/usr/bin/env node

import { existsSync } from "node:fs";
import { serve } from "../vendor/hono-server@1.3.1.mjs";
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

serve(
  { ...args, fetch: serveHot(args) },
  (info) => {
    console.log(`Listening on http://${args.host ?? "localhost"}:${info.port}`);
  },
);
