// https://nodejs.org/api/worker_threads.html

// copied from https://github.com/jspm/jspm-core/blob/main/src-browser/worker_threads.js

import { EventEmitter, once } from './node_events.js';

function unimplemented(name) {
  throw new Error(
    `Node.js worker_threads ${name} is not currently supported in the browser`,
  );
}

let environmentData = new Map();
let threads = 0;

const kHandle = Symbol('kHandle');
export class Worker extends EventEmitter {
  resourceLimits = {
    maxYoungGenerationSizeMb: -1,
    maxOldGenerationSizeMb: -1,
    codeRangeSizeMb: -1,
    stackSizeMb: 4,
  };

  constructor(specifier, options) {
    super();
    if (options?.eval === true) {
      specifier = URL.createObjectURL(new Blob([specifier], { type: 'application/javascript' }));
    }
    const handle = this[kHandle] = new globalThis.Worker(specifier, {
      ...(options || {}),
      type: 'module',
    });
    handle.addEventListener('error', (event) => this.emit('error', event.error || event.message));
    handle.addEventListener('messageerror', (event) => this.emit('messageerror', event.data));
    handle.addEventListener('message', (event) => this.emit('message', event.data));
    handle.postMessage({
      environmentData,
      threadId: (this.threadId = ++threads),
      workerData: options?.workerData,
    }, options?.transferList);
    this.postMessage = handle.postMessage.bind(handle);
    this.emit('online');
  }

  terminate() {
    this[kHandle].terminate();
    this.emit('exit', 0);
  }

  getHeapSnapshot = () => unimplemented('Worker#getHeapsnapshot');
  // fake performance
  performance = globalThis.performance;
}

export const isMainThread = typeof WorkerGlobalScope === 'undefined' || self instanceof WorkerGlobalScope === false;

// fake resourceLimits
export const resourceLimits = isMainThread ? {} : {
  maxYoungGenerationSizeMb: 48,
  maxOldGenerationSizeMb: 2048,
  codeRangeSizeMb: 0,
  stackSizeMb: 4,
};

let threadId = 0;
let workerData = null;
let parentPort = null;

if (!isMainThread) {
  const listeners = new WeakMap();
  parentPort = self;
  parentPort.off = parentPort.removeListener = function (name, listener) {
    this.removeEventListener(name, listeners.get(listener));
    listeners.delete(listener);
    return this;
  };
  parentPort.on = parentPort.addListener = function (name, listener) {
    const _listener = (ev) => listener(ev.data);
    listeners.set(listener, _listener);
    this.addEventListener(name, _listener);
    return this;
  };
  parentPort.once = function (name, listener) {
    const _listener = (ev) => listener(ev.data);
    listeners.set(listener, _listener);
    this.addEventListener(name, _listener);
    return this;
  };

  // mocks
  parentPort.setMaxListeners = () => {};
  parentPort.getMaxListeners = () => Infinity;
  parentPort.eventNames = () => [];
  parentPort.listenerCount = () => 0;

  parentPort.emit = () => notImplemented();
  parentPort.removeAllListeners = () => notImplemented();

  ([{ threadId, workerData, environmentData }] = await once(parentPort, 'message'));

  // alias
  parentPort.addEventListener('offline', () => parentPort.emit('close'));
}

export function getEnvironmentData(key) {
  return environmentData.get(key);
}

export function setEnvironmentData(key, value) {
  if (value === undefined) {
    environmentData.delete(key);
  } else {
    environmentData.set(key, value);
  }
}

export const markAsUntransferable = () => unimplemented('markAsUntransferable');
export const moveMessagePortToContext = () => unimplemented('moveMessagePortToContext');
export const receiveMessageOnPort = () => unimplemented('receiveMessageOnPort');
export const MessagePort = globalThis.MessagePort;
export const MessageChannel = globalThis.MessageChannel;
export const BroadcastChannel = globalThis.BroadcastChannel;
export const SHARE_ENV = Symbol.for('nodejs.worker_threads.SHARE_ENV');
export { parentPort, threadId, workerData }

export default {
  markAsUntransferable,
  moveMessagePortToContext,
  receiveMessageOnPort,
  MessagePort,
  MessageChannel,
  BroadcastChannel,
  Worker,
  getEnvironmentData,
  setEnvironmentData,
  SHARE_ENV,
  threadId,
  workerData,
  resourceLimits,
  parentPort,
  isMainThread,
}
