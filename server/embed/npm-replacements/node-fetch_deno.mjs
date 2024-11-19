export const { Blob, fetch, File, FormData, Headers, Request, Response, AbortController } = globalThis;
export const AbortError = Error;
export const FetchError = Error;
export const isRedirect = (code) => (code > 300 && code < 304) || (code > 306 && code < 309);

fetch.isRedirect = isRedirect;
fetch.Promise = globalThis.Promise;
export default fetch;

export async function blobFrom(path, type) {
  const file = await Deno.open(path);
  const res = new Response(file.readable);
  return new Blob([await res.blob()], { type });
}

export function blobFromSync(path, type) {
  const data = Deno.readFileSync(path);
  return new Blob([data], { type });
}

export async function fileFrom(path, type) {
  const file = await Deno.open(path);
  const res = new Response(file.readable);
  return new File([await res.blob()], path.split(/[\/\\]/).pop(), { type });
}

export function fileFromSync(path, type) {
  const data = Deno.readFileSync(path);
  return new File([data], path.split(/[\/\\]/).pop(), { type });
}
