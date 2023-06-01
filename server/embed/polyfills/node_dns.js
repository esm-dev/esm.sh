// https://nodejs.org/api/dns.html

function panic() {
  throw new Error(
    `[esm.sh] "node:dns" is not supported in browser environment.`,
  );
}

export class Resolver {
  constructor() {
    panic();
  }
}

export let promises = new Proxy({}, {
  get: (_t, prop) => notImplemented(`promises/${prop}`),
});

export function getServers() {
  panic();
}

export function lookup() {
  panic();
}

export function lookupService() {
  panic();
}

export function resolve() {
  panic();
}

export function resolve4() {
  panic();
}

export function resolve6() {
  panic();
}

export function resolveAny() {
  panic();
}

export function resolveCname() {
  panic();
}

export function resolveCaa() {
  panic();
}

export function resolveMx() {
  panic();
}

export function resolveNaptr() {
  panic();
}

export function resolveNs() {
  panic();
}

export function resolvePtr() {
  panic();
}

export function resolveSoa() {
  panic();
}

export function resolveSrv() {
  panic();
}

export function resolveTxt() {
  panic();
}

export function reverse() {
  panic();
}

export function setDefaultResultOrder() {
  panic();
}

export function setServers() {
  panic();
}

export default {
  Resolver,
  promises,
  getServers,
  lookup,
  lookupService,
  resolve,
  resolve4,
  resolve6,
  resolveAny,
  resolveCname,
  resolveCaa,
  resolveMx,
  resolveNaptr,
  resolveNs,
  resolvePtr,
  resolveSoa,
  resolveSrv,
  resolveTxt,
  reverse,
  setDefaultResultOrder,
  setServers,
};
