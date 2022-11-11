export const Blob = globalThis.Blob
export const File = globalThis.File
export const FormData = globalThis.FormData
export const Headers = globalThis.Headers
export const Request = globalThis.Request
export const Response = globalThis.Response
export const AbortController = globalThis.AbortController

export const fetch = globalThis.fetch || (() => { throw new Error('global fetch is not available!') })
export default fetch

export class AbortError extends Error { }
export class FetchError extends Error { }

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

const redirectStatus = new Set([301, 302, 303, 307, 308]);

/**
 * Redirect code matching
 *
 * @param {number} code - Status code
 * @return {boolean}
 */
export const isRedirect = code => {
  return redirectStatus.has(code);
};