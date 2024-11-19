export const { Blob, fetch, File, FormData, Headers, Request, Response, AbortController } = globalThis;
export const AbortError = Error;
export const FetchError = Error;
export const blobFrom = isNotSupport("blobFrom");
export const blobFromSync = isNotSupport("blobFromSync");
export const fileFrom = isNotSupport("fileFrom");
export const fileFromSync = isNotSupport("fileFromSync");
export const isRedirect = (code) => (code > 300 && code < 304) || (code > 306 && code < 309);

fetch.isRedirect = isRedirect;
fetch.Promise = globalThis.Promise;
export default fetch;

function isNotSupport(fn) {
  return () => {
    throw new Error(`${fn} is not supported in browser`);
  };
}
