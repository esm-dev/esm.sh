// https://nodejs.org/api/dns.html

function notImplemented(name, type = 'function') {
  throw new Error(`[esm.sh] dns: ${type} '${name}' is not implemented`)
}

export class Resolver {
  constructor() {
    notImplemented('Resolver', 'class')
  }
}

export let promises = new Proxy({}, { get: (_t, prop) => notImplemented(`promises/${prop}`) });

export function getServers() {
  notImplemented("getServers")
}

export function lookup() {
  notImplemented("lookup")
}

export function lookupService() {
  notImplemented("lookupService")
}

export function resolve() {
  notImplemented("resolve")
}

export function resolve4() {
  notImplemented("resolve4")
}

export function resolve6() {
  notImplemented("resolve6")
}

export function resolveAny() {
  notImplemented("resolveAny")
}

export function resolveCname() {
  notImplemented("resolveCname")
}

export function resolveCaa() {
  notImplemented("resolveCaa")
}

export function resolveMx() {
  notImplemented("resolveMx")
}

export function resolveNaptr() {
  notImplemented("resolveNaptr")
}

export function resolveNs() {
  notImplemented("resolveNs")
}

export function resolvePtr() {
  notImplemented("resolvePtr")
}

export function resolveSoa() {
  notImplemented("resolveSoa")
}

export function resolveSrv() {
  notImplemented("resolveSrv")
}

export function resolveTxt() {
  notImplemented("resolveTxt")
}

export function reverse() {
  notImplemented("reverse")
}

export function setDefaultResultOrder() {
  notImplemented("setDefaultResultOrder")
}

export function setServers() {
  notImplemented("setServers")
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
}
