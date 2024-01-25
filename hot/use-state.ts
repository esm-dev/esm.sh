import type { Hot } from "../server/embed/types/hot.d.ts";

const doc = document;
const obj = Object;
const symWatch = Symbol();
const symUnWatch = Symbol();
const symAnyKey = Symbol();

const htmlBuiltinBooleanAttrs = new Set([
  "allowfullscreen",
  "async",
  "autofocus",
  "autoplay",
  "checked",
  "controls",
  "default",
  "defer",
  "disabled",
  "formnovalidate",
  "inert",
  "ismap",
  "itemscope",
  "loop",
  "multiple",
  "muted",
  "nomodule",
  "novalidate",
  "open",
  "playsinline",
  "readonly",
  "required",
  "reversed",
  "selected",
]);

/** create a reactive store. */
function createStore<T extends object>(
  init: T,
  watchInit?: Iterable<[string | symbol, Set<() => void>]>,
): T {
  let filled = false;
  let effectPending = false;
  const effectKeys: Set<string | symbol> = new Set();
  const effect = () => {
    [...effectKeys, symAnyKey].forEach((key) => {
      watchers.get(key)?.forEach((handler) => {
        handler();
      });
    });
    effectKeys.clear();
    effectPending = false;
  };
  const watchers = new Map(watchInit);
  const watch = (key: string | symbol, handler: () => void) => {
    const set = watchers.get(key) ?? (watchers.set(key, new Set()).get(key)!);
    const add = () => set.add(handler);
    add();
    return () => { // dispose
      set.delete(handler);
      return add; // recover
    };
  };
  const unwatch = () => {
    const entries = watchers.entries();
    watchers.clear();
    return entries;
  };
  const isArray = Array.isArray(init);
  const store = new Proxy(isArray ? [] : Object.create(null), {
    get: (target, key) => {
      if (key === symWatch) {
        return watch;
      }
      if (key === symUnWatch) {
        return unwatch;
      }
      return get(target, key);
    },
    set: (target, key, value) => {
      const old = get(target, key);
      if (old === value) {
        return true;
      }
      if (isObject(value) && !get(value, symWatch)) {
        if (isObject(old)) {
          get(old, symUnWatch)?.();
        }
        value = createStore(value);
      }
      const oldLength = isArray ? target.length : 0;
      const ok = set(target, key, value);
      if (ok && filled) {
        if (isArray && oldLength !== target.length) {
          effectKeys.add("length");
        }
        effectKeys.add(key);
        if (!effectPending) {
          effectPending = true;
          // simple scheduler
          queueMicrotask(effect);
        }
      }
      return ok;
    },
  });
  for (const [key, value] of Object.entries(init)) {
    store[key] = value;
  }
  filled = true;
  return store;
}

/** split the given expression by blocks. */
function parseBlocks(
  text: string,
  blockStart = "{",
  blockEnd = "}",
): [(string | Expr)[], number] {
  const segments: (string | Expr)[] = [];
  let i = 0;
  let j = 0;
  let blocks = 0;
  while (i < text.length) {
    j = text.indexOf(blockStart, i);
    if (j === -1) {
      segments.push(text.slice(i));
      break;
    }
    if (j > i) {
      segments.push(text.slice(i, j));
    }
    i = text.indexOf(blockEnd, j);
    if (i === -1) {
      segments[segments.length - 1] += text.slice(j);
      break;
    }
    const seg = text.slice(j + blockStart.length, i);
    const trimmed = seg.trim();
    if (trimmed) {
      const e = tokenizeExpr(trimmed);
      if (e) {
        blocks++;
      }
      segments.push(e ?? seg);
    }
    i++;
  }
  return [segments, blocks];
}

/** find the first object that has the given property. */
function findOwn(list: any[], key: PropertyKey) {
  return list.find((o) => Reflect.has(o, key));
}

/** get the given property of the given target. */
function get(target: object, key: PropertyKey) {
  return Reflect.get(target, key);
}

/** set the given property of the given target. */
function set(target: object, key: PropertyKey, value: unknown) {
  return Reflect.set(target, key, value);
}

/** get the attribute value of the given element. */
function attr(el: Element, name: string, value?: string) {
  if (!isNullish(value)) {
    return el.setAttribute(name, value);
  }
  return el.getAttribute(name);
}

/** get all attribute names of the given element. */
function attrs(el: Element) {
  return el.getAttributeNames();
}

/** walk all nodes recursively. */
function walkNodes(
  { childNodes }: Element,
  handler: (node: ChildNode) => void | false,
) {
  const nodes = [...childNodes]; // copy the node list in case the handler updates it
  nodes.forEach((node) => {
    if (handler(node) !== false && node.nodeType === 1) {
      walkNodes(node as Element, handler);
    }
  });
}

/** check if the given value is nullish. */
function isNullish(v: unknown): v is null | undefined {
  return v === null || v === undefined;
}

/** check if the given value is an object. */
function isObject(v: unknown): v is object {
  return typeof v === "object" && v !== null;
}

/** check if the given value is a plain object. */
function isPlainObject(v: unknown): v is Record<string, unknown> {
  return isObject(v) && v.constructor === Object;
}

/** check if the given text is a block string. */
function isBlockExpr(text: string, blockStart = "{", blockEnd = "}") {
  const trimed = text.trim();
  return trimed.startsWith(blockStart) && trimed.endsWith(blockEnd);
}

/** convert the given value to string. */
function toString(value: unknown, skipBoolean = false) {
  if (isNullish(value) || (skipBoolean && typeof value === "boolean")) {
    return "";
  }
  if (typeof value === "string") {
    return value;
  }
  return (value as any).toString?.() ?? JSON.stringify(value);
}

type Expr = [
  ident: string,
  accesser: string | symbol | undefined,
  op: boolean,
  raw: string,
];

const regIdent = /^[a-zA-Z_$][\w$]+$/;
const regExpr =
  /^(!+)?([\w$]+)\s*(?:\.([\w$]+)|\[([\w$]+|'.+?'|".+?")\])?\s*([\[\^+\-*/%<>|&.!?].+)?$/;
const exprCache = new Map<string, Expr>();
const tokenizeExpr = (blockExpr: string) => {
  const expr = exprCache.get(blockExpr);
  if (expr) {
    return expr;
  }
  const m = blockExpr.match(regExpr);
  if (m) {
    const [_, preOp, ident, dotAcc, bracketAcc, postOp] = m;
    let accesser: Expr[1] = dotAcc;
    if (bracketAcc) {
      const c = bracketAcc.charCodeAt(0);
      accesser = c === 39 /* ' */ || c === 34 /* " */
        ? bracketAcc.slice(1, -1)
        : (regIdent.test(bracketAcc) ? symAnyKey : bracketAcc);
    }
    const expr: Expr = [
      ident,
      accesser,
      !!(preOp || accesser || postOp),
      blockExpr,
    ];
    exprCache.set(blockExpr, expr);
    return expr;
  }
  return null;
};

/** core logic of the <use-state> tag. */
function core(
  root: Element,
  globalState: Record<string, unknown> | null,
) {
  const inhertScopes = get(root, "$scopes") ??
    (globalState ? [globalState] : []);
  const init = new Function(
    "$scope",
    "return " + (attr(root, "onload") ?? "null"),
  )(
    new Proxy(Object.create(null), {
      get: (_, key) => findOwn(inhertScopes, key)?.[key],
      set: (_, key, value) => {
        const s = findOwn(inhertScopes, key) ?? inhertScopes[0];
        return s ? set(s, key, value) : false;
      },
    }),
  );
  const withProp = attr(root, "with");
  const withScope = withProp
    ? findOwn(inhertScopes, withProp)?.[withProp]
    : null;
  const scopes = [
    ...(isObject(init) ? [createStore(init)] : []),
    ...(isObject(withScope) ? [withScope] : []),
    ...inhertScopes,
  ];
  const interpret = (
    $scopes: Record<string, unknown>[],
    expr: string | [(string | Expr)[], number],
    update: (nextValue: unknown) => void,
    watch = true,
    blockStart?: string,
    blockEnd?: string,
  ) => {
    const [segments, blocks] = Array.isArray(expr)
      ? expr
      : parseBlocks(expr, blockStart, blockEnd);
    if (blocks === 0) {
      return; // no blocks
    }
    const createEffect = (expr: Expr, callback: (value: unknown) => void) => {
      const [ident, accesser, op, rawExpr] = expr;
      const scope = findOwn($scopes, ident) ?? $scopes[0];
      let dispose: (() => void) | undefined;
      const invoke = () => {
        dispose?.();
        const call = () => {
          const value = get(scope, ident);
          if (op) {
            callback(new Function(ident, "return " + rawExpr)(value));
          } else {
            callback(value);
          }
          return value;
        };
        const value = call();
        if (watch && accesser && isObject(value)) {
          dispose = get(value, symWatch)(accesser, call);
        }
      };
      invoke();
      if (watch) {
        get(scope, symWatch)?.(ident, invoke);
      }
    };
    // singleton block
    if (blocks === 1 && segments.length === 1) {
      return createEffect(segments[0] as Expr, update);
    }
    const invokedSegments = segments.map((seg, i) => {
      if (Array.isArray(seg)) {
        let blockValue: string | undefined;
        createEffect(seg, (v) => {
          const s = toString(v, true);
          if (!blockValue) {
            blockValue = s;
          } else {
            invokedSegments[i] = s;
            merge();
          }
        });
        return blockValue;
      }
      return seg;
    });
    const merge = () => update(invokedSegments.join(""));
    merge();
  };
  const reactive = (el: Element, currentScope?: unknown) => {
    let $scopes = scopes;
    if (currentScope) {
      $scopes = [currentScope, ...scopes];
    }
    const handler = (node: ChildNode) => {
      if (node.nodeType === 1 /* element node */) {
        const el = node as Element;
        const tagName = el.tagName.toLowerCase();
        const props = attrs(el);

        // nested <use-state> tag
        if (tagName === "use-state") {
          Object.assign(el, { $scopes });
          return false;
        }

        const boolAttrs = new Set<string>();
        const commonAttrs = new Set<string>();
        const eventAttrs = new Set<string>();

        for (const prop of props) {
          if (attr(el, prop) === "" && !htmlBuiltinBooleanAttrs.has(prop)) {
            boolAttrs.add(prop);
          } else if (prop.startsWith("on")) {
            eventAttrs.add(prop);
          } else {
            commonAttrs.add(prop);
          }
        }

        // list rendering
        if (commonAttrs.has("for")) {
          const [iter, iterArrIdent] = attr(el, "for")!.split(" of ").map((s) =>
            s.trim()
          );
          if (iter && iterArrIdent) {
            const templateEl = el;
            const keyProp = attr(templateEl, "key");
            const placeholder = doc!.createComment("&")!;
            const scope = findOwn($scopes, iterArrIdent) ?? $scopes[0];
            let marker: Element[] = [];
            let unwatch: (() => void) | undefined;
            const renderList = () => {
              unwatch?.(); // dispose the previous array watcher if exists
              const arr = get(scope, iterArrIdent);
              if (Array.isArray(arr)) {
                let iterIdent = iter;
                let iterIndexIdent = "";
                if (isBlockExpr(iterIdent, "(", ")")) {
                  [iterIdent, iterIndexIdent] = iterIdent.slice(1, -1)
                    .split(",", 2)
                    .map((s) => s.trim());
                }
                const render = () => {
                  const map = new Map<string, Element>();
                  for (const el of marker) {
                    const key = attr(el, "key");
                    key && map.set(key, el);
                  }
                  const listEls = arr.map((item, index) => {
                    const iterScope: Record<string, unknown> = {};
                    if (iterIdent) {
                      iterScope[iterIdent] = item;
                    }
                    if (iterIndexIdent) {
                      iterScope[iterIndexIdent] = index;
                    }
                    if (keyProp && map.size > 0) {
                      let key = "";
                      interpret(
                        [iterScope, ...$scopes],
                        keyProp,
                        (ret) => {
                          key = toString(ret);
                        },
                        false,
                      );
                      const sameKeyEl = map.get(key);
                      if (sameKeyEl) {
                        if (iterIndexIdent) {
                          get(sameKeyEl, "$scope")[iterIndexIdent] = index;
                        }
                        return sameKeyEl;
                      }
                    }
                    const listEl = templateEl.cloneNode(true) as Element;
                    const lterScopeStore = createStore(iterScope);
                    set(listEl, "$scope", lterScopeStore);
                    reactive(listEl, lterScopeStore);
                    return listEl;
                  });
                  marker.forEach((el) => el.remove());
                  listEls.forEach((el) => placeholder.before(el));
                  marker = listEls;
                };
                render();
                // watch the array changes and re-render
                unwatch = get(arr, symWatch)(symAnyKey, render);
              } else if (marker.length > 0) {
                marker.forEach((el) => el.remove());
                marker.length = 0;
              }
            };
            el.replaceWith(placeholder);
            templateEl.removeAttribute("for");
            renderList();
            scope[symWatch](iterArrIdent, renderList);
          }
          return false;
        }

        // render properties with state
        const style = [attr(el, "style"), null];
        for (const prop of commonAttrs) {
          const isStyle = prop === "style";
          interpret(
            $scopes,
            attr(el, prop) ?? "",
            (v) => {
              if (htmlBuiltinBooleanAttrs.has(prop)) {
                if (v) {
                  attr(el, prop, "");
                } else {
                  el.removeAttribute(prop);
                }
              } else {
                let propName = prop;
                let propValue = toString(v, true);
                if (isStyle) {
                  style[0] = propValue;
                } else if (propName === "+style") {
                  style[1] = propValue;
                  propName = "style";
                }
                if (propName === "style") {
                  propValue = style.filter(Boolean).join(";");
                }
                attr(el, propName, propValue);
              }
            },
            true,
            isStyle ? "state(" : undefined,
            isStyle ? ")" : undefined,
          );
        }

        // conditional rendering
        let cProp = "";
        let notOp = false;
        for (let prop of boolAttrs) {
          notOp = prop.startsWith("!");
          if (notOp) {
            prop = prop.slice(1);
          }
          if (findOwn($scopes, prop)) {
            cProp = prop;
            break;
          }
        }
        if (cProp) {
          const scope = findOwn($scopes, cProp);
          if (scope) {
            const cEl = el;
            const placeholder = doc!.createComment("&")!;
            let anchor: ChildNode = el;
            const switchEl = (nextEl: ChildNode) => {
              if (nextEl !== anchor) {
                anchor.replaceWith(nextEl);
                anchor = nextEl;
              }
            };
            const toggle = () => {
              let ok = get(scope!, cProp!);
              if (notOp) {
                ok = !ok;
              }
              if (ok) {
                switchEl(cEl);
              } else {
                switchEl(placeholder);
              }
            };
            toggle();
            scope[symWatch](cProp, toggle);
          }
        }

        // bind scopes for event handlers
        if (eventAttrs.size > 0) {
          const marker = new Set<string>();
          for (const scope of $scopes) {
            const keys = obj.keys(scope);
            for (const key of keys) {
              if (!marker.has(key)) {
                marker.add(key);
                if (!Object.hasOwn(el, key)) {
                  Object.defineProperty(el, key, {
                    get: () => get(scope, key),
                    set: (value) => set(scope, key, value),
                  });
                }
              }
            }
          }
          // apply event modifiers if exists
          for (const a of eventAttrs) {
            let handler = attr(el, a);
            if (handler) {
              const [event, ...rest] = a.toLowerCase().split(".");
              const modifiers = new Set(rest);
              const addCode = (code: string) => {
                handler = code + handler;
              };
              if (modifiers.size) {
                if (modifiers.delete("once")) {
                  addCode(`this.removeAttribute('${event}');`);
                }
                if (modifiers.delete("prevent")) {
                  addCode("event.preventDefault();");
                }
                if (modifiers.delete("stop")) {
                  addCode("event.stopPropagation();");
                }
                if (event.startsWith("onkey") && modifiers.size) {
                  if (modifiers.delete("space")) {
                    modifiers.add(" ");
                  }
                  addCode(
                    "if(![" +
                      ([...modifiers].map((k) =>
                        "'" + (k.length > 1
                          ? k.charAt(0).toUpperCase() + k.slice(1)
                          : k) +
                        "'"
                      ).join(",")) +
                      "].includes(event.key))return;",
                  );
                }
                el.removeAttribute(a);
                attr(el, event, handler);
              }
            }
          }
        }

        // bind state for input, select and textarea elements
        if (
          tagName === "input" ||
          tagName === "select" ||
          tagName === "textarea"
        ) {
          const inputEl = el as
            | HTMLInputElement
            | HTMLSelectElement
            | HTMLTextAreaElement;
          const name = attr(inputEl, "name");
          if (name) {
            const scope = findOwn($scopes, name);
            if (scope) {
              const type = attr(inputEl, "type");
              const isCheckBox = type === "checkbox";
              let getValue: () => unknown = () => inputEl.value;
              if (type === "number") {
                getValue = () => Number(inputEl.value);
              } else if (isCheckBox) {
                getValue = () => (inputEl as HTMLInputElement).checked;
              }
              const updateValue = () => {
                const value = get(scope, name);
                if (isCheckBox) {
                  (inputEl as HTMLInputElement).checked = !!value;
                } else {
                  inputEl.value = toString(value);
                }
              };
              updateValue();
              const dispose = scope[symWatch](name, updateValue);
              inputEl.addEventListener("input", () => {
                const recover = dispose();
                set(scope, name, getValue());
                recover();
              });
            }
          }
        }
      } else if (node.nodeType === 3 /* text node */) {
        const text = node as Text;
        const [segments, blocks] = parseBlocks(text.nodeValue!);
        if (blocks === 0) {
          return; // no blocks
        }
        const textNodes = segments.map((seg) => {
          if (Array.isArray(seg)) {
            const blockText = doc.createTextNode("");
            interpret(
              $scopes,
              [[seg], 1],
              (v) => blockText.nodeValue = toString(v, true),
            );
            return blockText;
          }
          return doc!.createTextNode(seg);
        });
        text.replaceWith(...textNodes.flat(1));
      }
    };
    if (el !== root) {
      handler(el);
    }
    walkNodes(el, handler);
  };
  reactive(root);
}

const plugin = {
  name: "use-state",
  setup(hot: Hot) {
    let globalState: Record<string, unknown> | null = null;

    hot.state = (
      init: Record<string, unknown> | Promise<Record<string, unknown>>,
    ): void => {
      if (init instanceof Promise) {
        hot.waitUntil(init.then((state) => hot.state(state)));
      } else if (isPlainObject(init)) {
        globalState = createStore(init);
      }
    };

    hot.onFire(() => {
      customElements.define(
        "use-state",
        class extends HTMLElement {
          connectedCallback() {
            core(this, globalState);
          }
        },
      );
      doc!.head.appendChild(doc!.createElement("style")).append(
        "use-state{visibility: visible;}",
      );
    });
  },
};

// use the plugin as a standalone module:
// https://esm.sh/hot/use-state?standalone
export function standalone() {
  const hot = {
    _promises: [] as Promise<void>[],
    _fire: () => {},
    waitUntil(promise: Promise<void>) {
      this._promises.push(promise);
    },
    onFire(fn: () => void) {
      this._fire = fn;
    },
  };
  plugin.setup(hot as unknown as Hot);
  return (
    init?: Record<string, unknown> | Promise<Record<string, unknown>>,
  ) => {
    init && (hot as unknown as Hot).state(init);
    if (hot._promises.length > 0) {
      Promise.all(hot._promises).then(() => hot._fire());
    } else {
      hot._fire();
    }
  };
}

export default plugin;
