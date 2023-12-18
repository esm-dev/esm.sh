/**
 * @hono/node-server v1.3.3
 * @author Yusuke Wada <https://github.com/yusukebe>
 * @license MIT
 *
 */

// src/server.ts
import { createServer as createServerHTTP } from "http";

// src/globals.ts
import crypto from "crypto";

// src/utils.ts
function writeFromReadableStream(stream, writable) {
  if (stream.locked) {
    throw new TypeError("ReadableStream is locked.");
  } else if (writable.destroyed) {
    stream.cancel();
    return;
  }
  const reader = stream.getReader();
  writable.on("close", cancel);
  writable.on("error", cancel);
  reader.read().then(flow, cancel);
  return reader.closed.finally(() => {
    writable.off("close", cancel);
    writable.off("error", cancel);
  });
  function cancel(error) {
    reader.cancel(error).catch(() => {
    });
    if (error)
      writable.destroy(error);
  }
  function onDrain() {
    reader.read().then(flow, cancel);
  }
  function flow({ done, value }) {
    try {
      if (done) {
        writable.end();
      } else if (!writable.write(value)) {
        writable.once("drain", onDrain);
      } else {
        return reader.read().then(flow, cancel);
      }
    } catch (e) {
      cancel(e);
    }
  }
}
var buildOutgoingHttpHeaders = (headers) => {
  const res = {};
  const cookies = [];
  for (const [k, v] of headers) {
    if (k === "set-cookie") {
      cookies.push(v);
    } else {
      res[k] = v;
    }
  }
  if (cookies.length > 0) {
    res["set-cookie"] = cookies;
  }
  res["content-type"] ??= "text/plain;charset=UTF-8";
  return res;
};

// src/response.ts
var responseCache = Symbol("responseCache");
var cacheKey = Symbol("cache");
var GlobalResponse = global.Response;
var Response2 = class _Response {
  #body;
  #init;
  get cache() {
    delete this[cacheKey];
    return this[responseCache] ||= new GlobalResponse(this.#body, this.#init);
  }
  constructor(body, init) {
    this.#body = body;
    if (init instanceof _Response) {
      const cachedGlobalResponse = init[responseCache];
      if (cachedGlobalResponse) {
        this.#init = cachedGlobalResponse;
        this.cache;
        return;
      } else {
        this.#init = init.#init;
      }
    } else {
      this.#init = init;
    }
    if (typeof body === "string" || body instanceof ReadableStream) {
      let headers = init?.headers || { "content-type": "text/plain;charset=UTF-8" };
      if (headers instanceof Headers) {
        headers = buildOutgoingHttpHeaders(headers);
      }
      ;
      this[cacheKey] = [init?.status || 200, body, headers];
    }
  }
};
[
  "body",
  "bodyUsed",
  "headers",
  "ok",
  "redirected",
  "status",
  "statusText",
  "trailers",
  "type",
  "url"
].forEach((k) => {
  Object.defineProperty(Response2.prototype, k, {
    get() {
      return this.cache[k];
    }
  });
});
["arrayBuffer", "blob", "clone", "formData", "json", "text"].forEach((k) => {
  Object.defineProperty(Response2.prototype, k, {
    value: function() {
      return this.cache[k]();
    }
  });
});
Object.setPrototypeOf(Response2, GlobalResponse);
Object.setPrototypeOf(Response2.prototype, GlobalResponse.prototype);
Object.defineProperty(global, "Response", {
  value: Response2
});

// src/globals.ts
Object.defineProperty(global, "Response", {
  value: Response2
});
var webFetch = global.fetch;
if (typeof global.crypto === "undefined") {
  global.crypto = crypto;
}
global.fetch = (info, init) => {
  init = {
    // Disable compression handling so people can return the result of a fetch
    // directly in the loader without messing with the Content-Encoding header.
    compress: false,
    ...init
  };
  return webFetch(info, init);
};

// src/request.ts
import { Readable } from "stream";
var newRequestFromIncoming = (method, url, incoming) => {
  const headerRecord = [];
  const len = incoming.rawHeaders.length;
  for (let i = 0; i < len; i += 2) {
    headerRecord.push([incoming.rawHeaders[i], incoming.rawHeaders[i + 1]]);
  }
  const init = {
    method,
    headers: headerRecord
  };
  if (!(method === "GET" || method === "HEAD")) {
    init.body = Readable.toWeb(incoming);
    init.duplex = "half";
  }
  return new Request(url, init);
};
var getRequestCache = Symbol("getRequestCache");
var requestCache = Symbol("requestCache");
var incomingKey = Symbol("incomingKey");
var requestPrototype = {
  get method() {
    return this[incomingKey].method || "GET";
  },
  get url() {
    return `http://${this[incomingKey].headers.host}${this[incomingKey].url}`;
  },
  [getRequestCache]() {
    return this[requestCache] ||= newRequestFromIncoming(this.method, this.url, this[incomingKey]);
  }
};
[
  "body",
  "bodyUsed",
  "cache",
  "credentials",
  "destination",
  "headers",
  "integrity",
  "mode",
  "redirect",
  "referrer",
  "referrerPolicy",
  "signal"
].forEach((k) => {
  Object.defineProperty(requestPrototype, k, {
    get() {
      return this[getRequestCache]()[k];
    }
  });
});
["arrayBuffer", "blob", "clone", "formData", "json", "text"].forEach((k) => {
  Object.defineProperty(requestPrototype, k, {
    value: function() {
      return this[getRequestCache]()[k]();
    }
  });
});
Object.setPrototypeOf(requestPrototype, global.Request.prototype);
var newRequest = (incoming) => {
  const req = Object.create(requestPrototype);
  req[incomingKey] = incoming;
  return req;
};

// src/listener.ts
var regBuffer = /^no$/i;
var regContentType = /^(application\/json\b|text\/(?!event-stream\b))/i;
var handleFetchError = (e) => new Response(null, {
  status: e instanceof Error && (e.name === "TimeoutError" || e.constructor.name === "TimeoutError") ? 504 : 500
});
var handleResponseError = (e, outgoing) => {
  const err = e instanceof Error ? e : new Error("unknown error", { cause: e });
  if (err.code === "ERR_STREAM_PREMATURE_CLOSE") {
    console.info("The user aborted a request.");
  } else {
    console.error(e);
    outgoing.destroy(err);
  }
};
var responseViaCache = (res, outgoing) => {
  const [status, body, header] = res[cacheKey];
  if (typeof body === "string") {
    header["Content-Length"] = Buffer.byteLength(body);
    outgoing.writeHead(status, header);
    outgoing.end(body);
  } else {
    outgoing.writeHead(status, header);
    return writeFromReadableStream(body, outgoing)?.catch(
      (e) => handleResponseError(e, outgoing)
    );
  }
};
var responseViaResponseObject = async (res, outgoing) => {
  if (res instanceof Promise) {
    res = await res.catch(handleFetchError);
  }
  if (cacheKey in res) {
    try {
      return responseViaCache(res, outgoing);
    } catch (e) {
      return handleResponseError(e, outgoing);
    }
  }
  const resHeaderRecord = buildOutgoingHttpHeaders(res.headers);
  if (res.body) {
    try {
      if (resHeaderRecord["transfer-encoding"] || resHeaderRecord["content-encoding"] || resHeaderRecord["content-length"] || // nginx buffering variant
      resHeaderRecord["x-accel-buffering"] && regBuffer.test(resHeaderRecord["x-accel-buffering"]) || !regContentType.test(resHeaderRecord["content-type"])) {
        outgoing.writeHead(res.status, resHeaderRecord);
        await writeFromReadableStream(res.body, outgoing);
      } else {
        const buffer = await res.arrayBuffer();
        resHeaderRecord["content-length"] = buffer.byteLength;
        outgoing.writeHead(res.status, resHeaderRecord);
        outgoing.end(new Uint8Array(buffer));
      }
    } catch (e) {
      handleResponseError(e, outgoing);
    }
  } else {
    outgoing.writeHead(res.status, resHeaderRecord);
    outgoing.end();
  }
};
var getRequestListener = (fetchCallback) => {
  return (incoming, outgoing) => {
    let res;
    const req = newRequest(incoming);
    try {
      res = fetchCallback(req);
      if (cacheKey in res) {
        return responseViaCache(res, outgoing);
      }
    } catch (e) {
      if (!res) {
        res = handleFetchError(e);
      } else {
        return handleResponseError(e, outgoing);
      }
    }
    return responseViaResponseObject(res, outgoing);
  };
};

// src/server.ts
var createAdaptorServer = (options) => {
  const fetchCallback = options.fetch;
  const requestListener = getRequestListener(fetchCallback);
  const createServer = options.createServer || createServerHTTP;
  const server = createServer(options.serverOptions || {}, requestListener);
  return server;
};
var serve = (options, listeningListener) => {
  const server = createAdaptorServer(options);
  server.listen(options?.port ?? 3e3, options.hostname ?? "0.0.0.0", () => {
    const serverInfo = server.address();
    listeningListener && listeningListener(serverInfo);
  });
  return server;
};
export {
  createAdaptorServer,
  getRequestListener,
  serve
};
