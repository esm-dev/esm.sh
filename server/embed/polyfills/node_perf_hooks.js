// https://nodejs.org/api/perf_hooks.html

function notImplemented(name) {
  throw new Error(`[esm.sh] pref_hooks: '${name}' is not implemented`)
}

export const performance = window.performance
export const PerformanceObserver = window.PerformanceObserver
export const PerformanceEntry = window.PerformanceEntry
export const PerformanceObserverEntryList = window.PerformanceObserverEntryList

export class PerformanceNodeTiming extends PerformanceEntry {
  constructor() {
    notImplemented('PerformanceNodeTiming')
  }
}

export class Histogram {
  constructor() {
    notImplemented('Histogram')
  }
}

export class IntervalHistogram extends Histogram {
  constructor() {
    notImplemented('IntervalHistogram')
  }
}

export class RecordableHistogram extends Histogram {
  constructor() {
    notImplemented('RecordableHistogram')
  }
}

export function createHistogram() {
  notImplemented('createHistogram')
}

export function monitorEventLoopDelay() {
  notImplemented('monitorEventLoopDelay')
}

export default {
  performance,
  PerformanceEntry,
  PerformanceNodeTiming,
  PerformanceObserver,
  PerformanceObserverEntryList,
  Histogram,
  IntervalHistogram,
  RecordableHistogram,
  createHistogram,
  monitorEventLoopDelay
}
