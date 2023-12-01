#!/usr/bin/env node

import { serve } from "@hono/node-server";
import { serveHot } from "../src/index.mjs";

if (process.argv.includes("--help") || process.argv.includes("-h")) {
  console.log(`
Usage: npx @esm.sh/hot [options]

Options:
  --cwd           Current working directory (default: ".")
  --help, -h      Show help
  --host          Host to listen on (default: "localhost")
  --plugins       Plugins for service worker (default: [])
  --port, -p      Port number to listen on (default: 3000)
  --spa           Enable SPA mode
  --watch         Watch file changes for HMR
`);
  process.exit(0);
}

const args = {
  port: 3000,
};

process.argv.slice(2).forEach((arg) => {
  const [key, value] = arg.split("=");
  if ((key === "--port" || key === "-p") && value) {
    args.port = parseInt(value);
    if (isNaN(args.port) && args.port <= 0) {
      throw new Error("Invalid port number");
    }
  } else if (key === "--host" && value) {
    args.host = value;
  } else if (key === "--spa") {
    if (value) {
      args.spa = { index: value };
    } else {
      args.spa = true;
    }
  } else if (key === "--watch" || key === "-w") {
    args.watch = true;
  } else if (key === "--cwd" && value) {
    args.cwd = value;
  } else if (key === "--plugins" && value) {
    args.plugins = value.split(",");
  }
});

serve(
  { ...args, fetch: serveHot(args) },
  (info) => {
    console.log(`Listening on http://${args.host ?? "localhost"}:${info.port}`);
  },
);
