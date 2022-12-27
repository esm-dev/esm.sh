// https://nodejs.org/api/dgram.html

function notImplemented(name, type = 'function') {
  throw new Error(`[esm.sh] dgram: ${type} '${name}' is not implemented`)
}

export class Socket {
  constructor(){
    notImplemented('Socket', 'class')
  }
}

export function createSocket(){
  notImplemented('createSocket')
}

export default{
  Socket,
  createSocket,
}