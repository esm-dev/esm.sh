import { open, readdir, stat, watch } from "node:fs/promises";
import { getMimeType } from "./mime.mjs";

const nameFilter = (filename) => {
  return !/(^|\/)(\.|node_modules\/)/.test(filename) &&
    !filename.endsWith(".log");
};

const fs = {
  /**
   * open a file.
   * @type {(path: string) => Promise<import("../types").FsFile>}
   */
  open: async (path) => {
    try {
      const file = await open(path, "r");
      const stat = await file.stat();
      return {
        size: stat.size,
        lastModified: stat.mtime.getTime(),
        contentType: getMimeType(path),
        body: file.readableWebStream(),
        close: () => file.close(),
      };
    } catch (error) {
      if (error.code === "ENOENT") {
        return null;
      }
      throw error;
    }
  },

  /**
   * find files in a directory.
   * @type {(dir: string) => Promise<string[]>}
   */
  ls: async (dir, parent) => {
    const files = [];
    const list = await readdir(dir, { withFileTypes: true });
    for (const entry of list) {
      const name = [parent, entry.name].filter(Boolean).join("/");
      if (entry.isDirectory()) {
        files.push(...(await fs.ls(dir + "/" + entry.name, name)));
      } else if (nameFilter(name)) {
        files.push(name);
      }
    }
    return files;
  },

  /**
   * watch for file changes.
   * @type {(root: string) => (handler: (type: string, filename: string)=>void) => () => void}
   */
  watch: (root) => {
    const watchCallbacks = new Set();
    const start = async () => {
      console.log("Watching for file changes...");
      for await (const evt of watch(root, { recursive: true })) {
        const { eventType, filename } = evt;
        if (nameFilter(filename)) {
          watchCallbacks.forEach((handler) =>
            handler(
              eventType === "change" ? "modify" : eventType,
              "/" + filename,
            )
          );
        }
      }
    };
    let started = false;
    return (handler) => {
      if (!started) {
        start();
        started = true;
      }
      watchCallbacks.add(handler);
      return () => {
        watchCallbacks.delete(handler);
      };
    };
  },
  stat: (path) => {
    return stat(path);
  },
};

if (typeof Deno !== "undefined") {
  fs.open = async (path) => {
    try {
      const file = await Deno.open(path);
      const stat = await file.stat();
      return {
        size: stat.size,
        lastModified: stat.mtime?.getTime() ?? null,
        contentType: getMimeType(path),
        body: file.readable,
        close: () => file.close(),
      };
    } catch (error) {
      if (error instanceof Deno.errors.NotFound) {
        return null;
      }
      throw error;
    }
  };
} else if (typeof Bun !== "undefined") {
  fs.open = async (path) => {
    const file = Bun.file(path);
    const found = await file.exists();
    if (!found) {
      return null;
    }
    return {
      size: file.size,
      lastModified: file.lastModified,
      contentType: getMimeType(path),
      body: await file.stream(),
      close: () => {},
    };
  };
}

export default fs;
