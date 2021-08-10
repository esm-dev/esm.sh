import queue from '/async/queue'

export async function test(el) {
  const q = queue(async (task, callback) => {
    const { name, testFn, li, span } = task
    const em = document.createElement('em')
    em.style.display = 'none'
    span.innerText = 'importing...'
    li.appendChild(em)

    try {
      const domain = localStorage.importDomain || '';
      const t1 = Date.now()
      const imports = Array.isArray(name) ? await Promise.all(name.map(n => {
        return import(`${domain}/${n}${n.includes('?') ? '&' : '?'}dev`)
      })) : await import(`${domain}/${name}${name.includes('?') ? '&' : '?'}dev`)
      const t2 = Date.now()

      try {
        await testFn({ imports, span })
      } catch (err) {
        span.innerText = `❌ ${err.message}`;
      }
      
      em.innerHTML = `&middot; import in <strong>${Math.round(t2 - t1)}</strong>ms`
      em.style.display = 'inline-block'
    } catch (e) {
      if (e.message.startsWith('[esm.sh] Unsupported nodejs builtin module')) {
        span.innerText = '⚠️ ' + e.message
      } else {
        span.innerText = '❌ ' + e.message
      }
    }

    callback() // invoke next task
  }, navigator.hardwareConcurrency || 1)

  const _esm = async (name, testFn) => {
    const li = document.createElement('li')
    const strong = document.createElement('strong')
    const span = document.createElement('span')
    const names = [name].flat()
    names.forEach((name, i) => {
      const a = document.createElement('a')
      a.innerText = name.split('?')[0]
      a.href = `/${name}${name.includes('?') ? '&' : '?'}dev`
      strong.appendChild(a)
      if (i < names.length - 1) {
        strong.appendChild(document.createTextNode(', '))
      }
    })
    strong.appendChild(document.createTextNode(':'))
    span.innerText = 'wait...'
    li.appendChild(strong)
    li.appendChild(span)
    el.appendChild(li)
    q.push({ name, testFn, li, span, })
  }

  _esm(['react@16', 'react-dom@16'], async (t) => {
    const [
      { createElement, Fragment, useState },
      { render }
    ] = t.imports

    const App = () => {
      const [count, setCount] = useState(0)
      return createElement(
        Fragment,
        null,
        createElement('span', null, '✅'),
        createElement('span', {
          onClick: () => setCount(n => n + 1),
          style: { cursor: 'pointer', userSelect: 'none' },
        }, ' ⏱ ', createElement('samp', null, count)),
      )
    }
    render(createElement(App), t.span)
  })

  _esm(['react@17', 'react-dom@17'], async (t) => {
    const [
      { Fragment, useState, default: React },
      { render }
    ] = t.imports

    const App = () => {
      const [count, setCount] = useState(0)
      return React.createElement(
        Fragment,
        null,
        React.createElement('span', null, '✅'),
        React.createElement('span', {
          onClick: () => setCount(n => n + 1),
          style: { cursor: 'pointer', userSelect: 'none' },
        }, ' ⏱ ', React.createElement('samp', null, count)),
      )
    }
    render(React.createElement(App), t.span)
  })

  _esm(['react@17', 'react-dom@17', 'react-redux?deps=react@17', 'redux'], async (t) => {
    const [
      { createElement, Fragment },
      { render },
      { Provider, useDispatch, useSelector },
      { createStore }
    ] = t.imports

    const store = createStore((state = { ok: '✅', count: 0 }, action) => {
      if (action.type === '+') {
        return { ...state, count: state.count + 1 }
      }
      return state
    })
    const App = () => {
      const ok = useSelector(state => state.ok)
      const count = useSelector(state => state.count)
      const dispatch = useDispatch()
      return createElement(
        Fragment,
        null,
        createElement('span', null, ok),
        createElement('span', {
          onClick: () => dispatch({ type: '+' }),
          style: { cursor: 'pointer', userSelect: 'none' },
        }, ' ⏱ ', createElement('samp', null, count)),
      )
    }
    render(createElement(Provider, { store }, createElement(App)), t.span)
  })

  _esm(['react@17', 'react-dom@17', 'mobx-react-lite?deps=react@17', 'mobx'], async (t) => {
    const [
      { createElement, Fragment },
      { render },
      { observer },
      { makeAutoObservable }
    ] = t.imports

    const store = makeAutoObservable({
      ok: '✅',
      count: 0,
    })
    const App = observer(({ store }) => {
      return createElement(
        Fragment,
        null,
        createElement('span', null, store.ok),
        createElement('span', {
          onClick: () => store.count++,
          style: { cursor: 'pointer', userSelect: 'none' },
        }, ' ⏱ ', createElement('samp', null, store.count))
      )
    })
    render(createElement(App, { store }), t.span)
  })

  _esm(['react@17', 'react-dom@17', 'antd?bundle'], async (t) => {
    const [
      { createElement, Fragment },
      { render },
      { Spin }
    ] = t.imports

    // spin style
    const styleEl = document.createElement('style')
    styleEl.innerHTML = `.ant-spin{box-sizing:border-box;margin:0;padding:0;color:#000000d9;font-size:14px;font-variant:tabular-nums;line-height:1.5715;list-style:none;font-feature-settings:"tnum";position:absolute;display:none;color:#1890ff;text-align:center;vertical-align:middle;opacity:0;transition:transform .3s cubic-bezier(.78,.14,.15,.86)}.ant-spin-spinning{position:static;display:inline-block;opacity:1}.ant-spin-dot{position:relative;display:inline-block;font-size:20px;width:1em;height:1em}.ant-spin-dot-item{position:absolute;display:block;width:9px;height:9px;background-color:#1890ff;border-radius:100%;transform:scale(.75);transform-origin:50% 50%;opacity:.3;-webkit-animation:antSpinMove 1s infinite linear alternate;animation:antSpinMove 1s infinite linear alternate}.ant-spin-dot-item:nth-child(1){top:0;left:0}.ant-spin-dot-item:nth-child(2){top:0;right:0;-webkit-animation-delay:.4s;animation-delay:.4s}.ant-spin-dot-item:nth-child(3){right:0;bottom:0;-webkit-animation-delay:.8s;animation-delay:.8s}.ant-spin-dot-item:nth-child(4){bottom:0;left:0;-webkit-animation-delay:1.2s;animation-delay:1.2s}.ant-spin-dot-spin{transform:rotate(45deg);-webkit-animation:antRotate 1.2s infinite linear;animation:antRotate 1.2s infinite linear}.ant-spin-sm .ant-spin-dot{font-size:14px}.ant-spin-sm .ant-spin-dot i{width:6px;height:6px}@-webkit-keyframes antSpinMove{to{opacity:1}}@keyframes antSpinMove{to{opacity:1}}@-webkit-keyframes antRotate{to{transform:rotate(405deg)}}@keyframes antRotate{to{transform:rotate(405deg)}}`
    document.head.appendChild(styleEl)

    const App = () => {
      return createElement(
        Fragment,
        null,
        createElement('span', null, '✅'),
        ' ',
        createElement(Spin, { size: 'small' }),
      )
    }
    render(createElement(App), t.span)
  })

  _esm(['preact', 'preact/hooks'], async (t) => {
    const [
      { Fragment, h, render },
      { useEffect, useState }
    ] = t.imports

    const App = () => {
      const [count, setCount] = useState(0)
      useEffect(() => {
        t.span.removeChild(t.span.lastChild)
      }, [])
      return h(
        Fragment,
        null,
        h('span', null, '✅'),
        h('span', {
          onClick: () => setCount(n => n + 1),
          style: { cursor: 'pointer', userSelect: 'none' },
        }, ' ⏱ ', h('samp', null, count))
      )
    }
    render(h(App), t.span)
  })

  _esm(['preact', 'preact/hooks', 'swr?alias=react:preact/compat'], async (t) => {
    const [
      { Fragment, h, render },
      { useEffect },
      { default: useSWR }
    ] = t.imports

    const App = () => {
      const { data, error } = useSWR('/status.json')
      useEffect(() => {
        t.span.removeChild(t.span.lastChild)
      }, [])
      return h(
        Fragment,
        null,
        error && h('span', null, 'failed to load'),
        !data && h('span', null, 'loading...'),
        data && h('span', null, '✅', ' data: ', h('code', null, JSON.stringify(Object.keys(data)))),
      )
    }
    render(h(App), t.span)
  })

  _esm('vue@2', async (t) => {
    const { default: Vue } = t.imports

    new Vue({
      el: t.span,
      data: { count: 0 },
      methods: {
        onClick() {
          this.count++
        }
      },
      render(h) {
        return h(
          'span',
          {},
          [
            h('span', {}, '✅'),
            h(
              'span',
              {
                style: { cursor: 'pointer', userSelect: 'none' },
                on: { click: this.onClick },
              },
              [' ⏱ ', h('samp', {}, this.count)]
            )
          ]
        )
      }
    })
  })

  _esm('vue@3', async (t) => {
    const { createApp, h } = t.imports

    createApp({
      data() {
        return { count: 0 }
      },
      methods: {
        onClick() {
          this.count++
        }
      },
      render() {
        return [
          h('span', {}, '✅'),
          h(
            'span',
            {
              style: { cursor: 'pointer', userSelect: 'none' },
              onClick: this.onClick,
            },
            ' ⏱ ',
            h('samp', {}, this.count),
          )
        ]
      }
    }).mount(t.span)
  })

  _esm('jquery', async (t) => {
    const { default: $ } = t.imports

    $(t.span).text('✅')
  })

  _esm('lodash', async (t) => {
    const { default: _ } = t.imports

    const defaults = _.defaults({ ok: '✅' }, { ok: '❌' })
    t.span.innerText = defaults.ok
  })

  _esm('d3', async (t) => {
    const d3 = t.imports

    t.span.id = 'd3-span'
    d3.select('#d3-span').text('✅')
  })
}
