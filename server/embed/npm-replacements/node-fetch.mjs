export const { Blob, fetch, File, FormData, Headers, Request, Response, AbortController } = globalThis;
fetch.Promise = globalThis.Promise;
fetch.isRedirect = isRedirect;
export default fetch;

export const AbortError = Error;
export const FetchError = Error;

const redirectStatus = /* @__PURE__ */ new Set([301, 302, 303, 307, 308]);
export const isRedirect = (code) => redirectStatus.has(code);

export async function blobFrom(path, type) {
  if (typeof Deno !== "undefined") {
    const file = await Deno.open(path);
    const res = new Response(file.readable);
    return new Blob([await res.blob()], { type });
  }
  throw new Error("blobFrom is not supported in browser");
}

export function blobFromSync(path, type) {
  if (typeof Deno !== "undefined") {
    const data = Deno.readFileSync(path);
    return new Blob([data], { type });
  }
  throw new Error("blobFromSync is not supported in browser");
}

export async function fileFrom(path, type) {
  if (typeof Deno !== "undefined") {
    const file = await Deno.open(path);
    const res = new Response(file.readable);
    return new File([await res.blob()], path.split(/[\/\\]/).pop(), { type });
  }
  throw new Error("blobFrom is not supported in browser");
}

export function fileFromSync(path, type) {
  if (typeof Deno !== "undefined") {
    const data = Deno.readFileSync(path);
    return new File([data], path.split(/[\/\\]/).pop(), { type });
  }
  throw new Error("blobFromSync is not supported in browser");
}
