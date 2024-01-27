import type { Hot } from "../server/embed/types/hot.d.ts";

const doc = document;
const obj = Object;
const symWatch = Symbol();
const symUnWatch = Symbol();
const symAnyKey = Symbol();

/** HTML built-in boolean attributes. */
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
      return add; // rewatch
    };
  };
  const unwatch = () => {
    const entries = watchers.entries();
    watchers.clear();
    return entries;
  };
  const isArray = Array.isArray(init);
  const store = new Proxy(isArray ? [] : obj.create(null), {
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
  for (const [key, value] of obj.entries(init)) {
    store[key] = value;
  }
  filled = true;
  return store;
}

/** split the given text by block expressions. */
function parseBlocks(
  text: string,
  blockStart = "{",
  blockEnd = "}",
): [segments: (string | Expr)[], blocks: number] {
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

function hasOwn(target: object, key: PropertyKey) {
  // @ts-ignore Object.hasOwn
  return obj.hasOwn ?? obj.hasOwnProperty.call(target, key);
}

/** find the first object that has the given property. */
function findOwn(list: any[], key: PropertyKey) {
  return list.find((o) => hasOwn(o, key));
}

/** get the given property of the given target. */
function get(target: object, key: PropertyKey) {
  return Reflect.get(target, key);
}

/** set the given property of the given target. */
function set(target: object, key: PropertyKey, value: unknown) {
  return Reflect.set(target, key, value);
}

/** get the tag name of the given element. */
function tagname(el: Element) {
  return el.tagName.toLowerCase();
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
  prop: string | symbol | undefined,
  op: boolean,
  raw: string,
];

const regIdent = /^[a-zA-Z_$][\w$]+$/;
const regExpr =
  /^(!*)([\w$]+)\s*(?:\.\s*([\w$]+)|\[\s*([\w$]+|'.+?'|".+?")\s*\])?\s*([\[\^+\-*/%<>=|&.!?].+)?$/;
const exprCache = new Map<string, Expr>();
const tokenizeExpr = (blockExpr: string) => {
  const expr = exprCache.get(blockExpr);
  if (expr) {
    return expr;
  }
  const m = regIdent.test(blockExpr)
    ? ["", "", blockExpr]
    : blockExpr.match(regExpr);
  if (m) {
    const [_, preOp, ident, dotProp, bracketProp, postOp] = m;
    let prop: Expr[1] = dotProp;
    if (bracketProp) {
      const c = bracketProp.charCodeAt(0);
      prop = c === 39 /* ' */ || c === 34 /* " */
        ? bracketProp.slice(1, -1)
        : (regIdent.test(bracketProp) ? symAnyKey : bracketProp);
    }
    const expr: Expr = [
      ident,
      prop,
      !!(preOp || prop || postOp),
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
  const parent = root.parentElement;
  if (parent && tagname(parent) === "use-content") {
    const fec = parent.firstElementChild;
    if (
      fec && tagname(fec) === "script" &&
      attr(fec, "type") === "application/json"
    ) {
      inhertScopes.unshift(createStore(JSON.parse(fec.textContent!)));
    }
  }
  const init = new Function(
    "$scope",
    "return " + (attr(root, "onload") ?? attr(root, "init") ?? "null"),
  )(
    new Proxy(obj.create(null), {
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
      const [ident, prop, op, rawExpr] = expr;
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
        if (watch && prop && isObject(value)) {
          dispose = get(value, symWatch)(prop, call);
        }
      };
      invoke();
      if (watch) {
        get(scope, symWatch)(ident, invoke);
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
  const statify = (el: Element, currentScope?: unknown) => {
    let $scopes = scopes;
    if (currentScope) {
      $scopes = [currentScope, ...scopes];
    }
    // bind scopes for event handlers
    const bindEventScope = (el: Element) => {
      const marker = new Set<string>();
      for (const scope of $scopes) {
        const keys = obj.keys(scope);
        for (const key of keys) {
          if (!marker.has(key)) {
            marker.add(key);
            if (!hasOwn(el, key)) {
              obj.defineProperty(el, key, {
                get: () => get(scope, key),
                set: (value) => set(scope, key, value),
              });
            }
          }
        }
      }
    };
    // apply event modifiers if exists
    const applyEventModifiers = (el: Element, eventAttrs: Set<string>) => {
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
    };
    // conditional rendering
    const conditionalRender = (el: Element, ident: string, notOp?: boolean) => {
      const scope = findOwn($scopes, ident);
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
          let ok = get(scope, ident);
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
        scope[symWatch](ident, toggle);
      }
    };
    // list rendering by checking the "for" attribute
    const renderList = (el: Element) => {
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
        const rerender = () => {
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
                statify(listEl, lterScopeStore);
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
        templateEl.removeAttribute("for");
        el.replaceWith(placeholder);
        rerender();
        scope[symWatch](iterArrIdent, rerender);
      }
    };
    // render text nodes
    const renderTextNode = (text: Text) => {
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
    };
    // render properties
    const renderProperties = (el: Element, attrNames: Set<string>) => {
      const style = [attr(el, "style"), null];
      for (const prop of attrNames) {
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
    };
    // two-way binding for <input> elements
    const createModel = (inputEl: HTMLInputElement) => {
      const name = attr(inputEl, "name");
      if (name) {
        const [ident, ...rest] = name.split(".");
        const scope = findOwn($scopes, ident);
        if (scope && rest.length < 2) {
          const subIdent = rest[0];
          const getValue = () => {
            const type = attr(inputEl, "type");
            if (type === "number" || type === "range") {
              return Number(inputEl.value);
            } else if (type === "checkbox") {
              return inputEl.checked;
            }
            return inputEl.value;
          };
          let subUnwatch: (() => () => void) | undefined;
          const bindValue = () => {
            subUnwatch?.();
            const updateEl = () => {
              const type = attr(inputEl, "type");
              let value = get(scope, ident);
              if (subIdent) {
                value = isObject(value) ? get(value, subIdent) : undefined;
              }
              if (type === "radio") {
                inputEl.checked = value === inputEl.value;
              } else if (type === "checkbox") {
                inputEl.checked = !!value;
              } else {
                inputEl.value = toString(value);
              }
            };
            updateEl();
            if (subIdent) {
              const v = get(scope, ident);
              if (isObject(v)) {
                subUnwatch = get(v, symWatch)(subIdent, updateEl);
              }
            }
          };
          bindValue();
          const unwatch = scope[symWatch](ident, bindValue);
          inputEl.addEventListener("input", () => {
            if (subIdent) {
              const v = get(scope, ident);
              if (isObject(v)) {
                const rewatch = subUnwatch?.();
                set(v, subIdent, getValue());
                rewatch?.();
              }
            } else {
              const rewatch = unwatch();
              set(scope, ident, getValue());
              rewatch();
            }
          });
        }
      }
    };
    // node handler
    const handler = (node: ChildNode) => {
      if (node.nodeType === 1 /* element node */) {
        const el = node as Element;
        const tagName = tagname(el);
        const props = attrs(el);

        // nested <use-state> tag
        if (tagName === "use-state") {
          obj.assign(el, { $scopes });
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
          renderList(el);
          return false;
        }

        // render properties
        renderProperties(el, commonAttrs);

        // conditional rendering
        let conditionalIdent = "";
        let notOp = false;
        for (let prop of boolAttrs) {
          notOp = prop.startsWith("!");
          if (notOp) {
            prop = prop.slice(1);
          }
          if (findOwn($scopes, prop)) {
            conditionalIdent = prop;
            break;
          }
        }
        if (conditionalIdent) {
          conditionalRender(el, conditionalIdent, notOp);
        }

        // bind scopes for event handlers
        if (eventAttrs.size > 0) {
          bindEventScope(el);
          applyEventModifiers(el, eventAttrs);
        }

        if (
          tagName === "input" ||
          tagName === "select" ||
          tagName === "textarea"
        ) {
          createModel(el as HTMLInputElement);
        }
      } else if (node.nodeType === 3 /* text node */) {
        renderTextNode(node as Text);
      }
    };
    if (el !== root) {
      handler(el);
    }
    walkNodes(el, handler);
  };
  statify(root);
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
            this.style.visibility = "visible";
          }
        },
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
