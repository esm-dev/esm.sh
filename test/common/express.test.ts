import { assertEquals } from "jsr:@std/assert";

import express, { type Request, type Response } from "http://localhost:8080/express@4";

Deno.test(
  "express",
  { sanitizeOps: false, sanitizeResources: false },
  async () => {
    const app = express();
    app.get("/", (_req: Request, res: Response) => {
      // @ts-ignore
      res.send("Hello World");
    });

    const ac = new AbortController();
    await new Promise<void>((resolve, reject) => {
      const server = app.listen({ port: 3333 }, (err?: Error) => {
        if (err) reject(err);
        resolve();
      });
      ac.signal.onabort = () => {
        server.close();
      };
    });

    const res = await fetch("http://localhost:3333");
    assertEquals(await res.text(), "Hello World");
    ac.abort();
  },
);
