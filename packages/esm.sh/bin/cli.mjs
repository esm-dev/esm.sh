#!/usr/bin/env node

import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { serve } from "../vendor/hono-server@1.3.3.mjs";
import { createESApp } from "../node.mjs";

// - Show help message
if (process.argv.includes("--help") || process.argv.includes("-h")) {
  const message = `
Usage: npx esm.sh [options] [dir]

Options:
  --help, -h      Show help message
  --host          Host to listen on (default: "localhost")
  --port, -p      Port number to listen on (default: 3000)
`;
  console.log(message);
  process.exit(0);
}

// - Parse command line arguments
const args = { port: 3000 };
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

// - Load project '.env' vars if exists
let dotEnvPath = join(args.root ?? process.cwd(), ".env.local");
if (!existsSync(dotEnvPath)) {
  dotEnvPath = join(args.root ?? process.cwd(), ".env");
}
if (existsSync(dotEnvPath)) {
  let section = "";
  const env = Object.fromEntries(
    readFileSync(dotEnvPath, "utf-8")
      .split("\n").map((line) => line.trim())
      .filter((line) => line && !line.startsWith("#"))
      .map((line) => {
        if (line.startsWith("[") && line.endsWith("]")) {
          section = line;
          return null;
        }
        const idx = line.indexOf("=");
        if (idx <= 0) {
          return null;
        }
        const key = (section ?? "") + line.slice(0, idx).trimEnd();
        const value = line.slice(idx + 1).trimStart();
        const v0 = value.charAt(0);
        let start = 0;
        let end = value.length;
        let endAt = "#";
        if (v0 === '"' || v0 === "'") {
          endAt = v0;
          start = 1;
        }
        const i = value.indexOf(endAt, 1);
        if (i >= 0) {
          end = i;
        }
        return [key, value.slice(start, end)];
      }).filter(Boolean),
  );
  Object.assign(process.env, env);
  console.log("Project '.env' loaded");
}

// - Start server
const esApp = createESApp(args);
serve(
  { ...args, fetch: esApp.fetch },
  (info) => {
    console.log(`Listening on http://${args.host ?? "localhost"}:${info.port}`);
  },
);
