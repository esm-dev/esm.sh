import queue from '/async/queue'

/*
  test example:
  ```js
  // single module
  test('packageName', async (t) => {
    const mod = t.module           // imported module
    t.$span.innterText = ':)'      // render testing content
    t.ok()                         // display '✅' and timing
  })

  // mulitple modules
  test(['packageName1', 'packageName2'], async (t) => {
    const [mod1, mod2] = t.modules // imported modules
    t.$span.innterText = ':)'      // render testing content
    t.ok()                         // display '✅' and timing
  })
  ```
*/

const q = queue(async ({ imports, testFn, $li, $status }) => {
  const $span = document.createElement('span')
  const $t = document.createElement('em')
  const start = Date.now()

  $status.innerHTML = 'importing...'
  $li.appendChild($span)

  try {
    const modules = Array.isArray(imports) ? await Promise.all(imports.map(n => {
      return import(`/${n}`)
    })) : []
    const module = typeof imports === 'string' ? await import(`/${imports}`) : undefined

    try {
      await testFn({ $span, modules, module, ok: () => $status.innerText = '✅' })
    } catch (err) {
      console.error(err.stack)
      $status.innerText = `❌ ${err.message}`;
    }

    $t.innerHTML = `&middot; import in <strong>${Math.round(Date.now() - start)}</strong>ms`
    $li.appendChild($t)
  } catch (e) {
    if (e.message.startsWith('[esm.sh] Unsupported nodejs builtin module')) {
      $status.innerText = '⚠️ ' + e.message
    } else {
      console.error(e.stack)
      $status.innerText = '❌ ' + e.message
    }
  }
}, 4)

const test = async (imports, testFn) => {
  const $li = document.createElement('li')
  const $imports = document.createElement('strong')
  const $status = document.createElement('span')
  const a = [imports].flat()

  a.forEach((name, i) => {
    const $a = document.createElement('a')
    $a.innerText = name.split('?')[0]
    $a.href = `./${name}${name.includes('?') ? '&' : '?'}dev`
    $imports.appendChild($a)
    if (i < a.length - 1) {
      $imports.appendChild(document.createTextNode(', '))
    }
  })
  $imports.appendChild(document.createTextNode(':'))
  $status.innerHTML = '<em>waiting...</em>'
  $li.appendChild($imports)
  $li.appendChild($status)
  $container.firstChild.appendChild($li)
  q.push({ imports, testFn, $li, $status })
}

// init dom
const $container = document.createElement('div')
$container.className = 'test'
$container.innerHTML = '<ul></ul><p class="link"><a href="./">&larr; Back </a></p>'
document.querySelector('h1 > em').appendChild(document.createTextNode(' · Testing'))
document.querySelector('main').remove()
document.querySelector('#root').appendChild($container)

test('canvas-confetti', async (t) => {
  const { default: confetti } = t.module

  t.$span.style.cursor = 'pointer'
  t.$span.style.userSelect = 'none'
  t.$span.addEventListener('click', () => confetti())
  t.$span.innerText = ' 🎉 '
  confetti()

  t.ok()
})

test(['react@16', 'react-dom@16'], async (t) => {
  const [
    { createElement, Fragment, useState },
    { render }
  ] = t.modules

  const App = () => {
    const [count, setCount] = useState(0)
    return createElement(
      Fragment,
      null,
      createElement('span', {
        onClick: () => setCount(n => n + 1),
        style: { cursor: 'pointer', userSelect: 'none' },
      }, '⏱ ', createElement('samp', null, count)),
    )
  }
  render(createElement(App), t.$span)

  t.ok()
})

test(['react@17', 'react-dom@17'], async (t) => {
  const [
    { useState, default: React },
    { render }
  ] = t.modules

  const App = () => {
    const [count, setCount] = useState(0)
    return React.createElement(
      React.Fragment,
      null,
      React.createElement('span', {
        onClick: () => setCount(n => n + 1),
        style: { cursor: 'pointer', userSelect: 'none' },
      }, '⏱ ', React.createElement('samp', null, count)),
    )
  }
  render(React.createElement(App), t.$span)

  t.ok()
})

test(['react@17', 'react-dom@17', 'react-redux?deps=react@17', 'redux'], async (t) => {
  const [
    { createElement, Fragment },
    { render },
    { Provider, useDispatch, useSelector },
    { createStore }
  ] = t.modules

  const store = createStore((state = { count: 0 }, action) => {
    if (action.type === '+') {
      return { ...state, count: state.count + 1 }
    }
    return state
  })
  const App = () => {
    const count = useSelector(state => state.count)
    const dispatch = useDispatch()
    return createElement(
      Fragment,
      null,
      createElement('span', {
        onClick: () => dispatch({ type: '+' }),
        style: { cursor: 'pointer', userSelect: 'none' },
      }, '⏱ ', createElement('samp', null, count)),
    )
  }
  render(createElement(Provider, { store }, createElement(App)), t.$span)

  t.ok()
})

test(['react@17', 'react-dom@17', 'mobx-react-lite?deps=react@17,react-dom@17', 'mobx'], async (t) => {
  const [
    { createElement, Fragment },
    { render },
    { observer },
    { makeAutoObservable }
  ] = t.modules

  const store = makeAutoObservable({
    count: 0,
  })
  const App = observer(({ store }) => {
    return createElement(
      Fragment,
      null,
      createElement('span', {
        onClick: () => store.count++,
        style: { cursor: 'pointer', userSelect: 'none' },
      }, '⏱ ', createElement('samp', null, store.count))
    )
  })
  render(createElement(App, { store }), t.$span)

  t.ok()
})

test(['react@17', 'react-dom@17', '@emotion/styled?deps=react@17', '@emotion/react?deps=react@17'], async (t) => {
  const [
    { createElement, Fragment, useState },
    { render },
    { default: styled },
    { Global, css }
  ] = t.modules

  const globalStyles = css`
    @keyframes breathing {
      0% {
        transform: scale(0.8);
      }
      25% {
        transform: scale(1.1);
      }
      60% {
        transform: scale(0.8);
      }
      100% {
        transform: scale(0.8);
      }
    }
  `
  const Span = styled.span`
    display: inline-block;
    animation: breathing 5s ease-out infinite normal;
    cursor: pointer;
    user-select: none;
  `

  const App = () => {
    const [count, setCount] = useState(0)
    return createElement(
      Fragment,
      null,
      createElement(Global, { styles: globalStyles }),
      createElement(Span, {
        onClick: () => setCount(n => n + 1),
      }, '⏱ ', createElement('samp', null, count)),
    )
  }
  render(createElement(App), t.$span)

  t.ok()
})

test(['react@17', 'react-dom@17', 'html-to-react?deps=react@17'], async (t) => {
  const [
    { createElement },
    { render },
    { Parser }
  ] = t.modules

  const h = new Parser()
  const App = () => {
    return h.parse(`<span>html to <strong>react</strong> is amzing</span>`)
  }
  render(createElement(App), t.$span)

  t.ok()
})

test(['react@17', 'react-dom@17', 'antd?deps=react@17,react-dom@17&bundle'], async (t) => {
  const [
    { createElement, Fragment },
    { render },
    { Spin }
  ] = t.modules

  // spin style
  const styleEl = document.createElement('style')
  styleEl.innerHTML = `.ant-spin{box-sizing:border-box;margin:0;padding:0;color:#000000d9;font-size:14px;font-variant:tabular-nums;line-height:1.5715;list-style:none;font-feature-settings:"tnum";position:absolute;display:none;color:#1890ff;text-align:center;vertical-align:middle;opacity:0;transition:transform .3s cubic-bezier(.78,.14,.15,.86)}.ant-spin-spinning{position:static;display:inline-block;opacity:1}.ant-spin-dot{position:relative;display:inline-block;font-size:20px;width:1em;height:1em}.ant-spin-dot-item{position:absolute;display:block;width:9px;height:9px;background-color:#1890ff;border-radius:100%;transform:scale(.75);transform-origin:50% 50%;opacity:.3;-webkit-animation:antSpinMove 1s infinite linear alternate;animation:antSpinMove 1s infinite linear alternate}.ant-spin-dot-item:nth-child(1){top:0;left:0}.ant-spin-dot-item:nth-child(2){top:0;right:0;-webkit-animation-delay:.4s;animation-delay:.4s}.ant-spin-dot-item:nth-child(3){right:0;bottom:0;-webkit-animation-delay:.8s;animation-delay:.8s}.ant-spin-dot-item:nth-child(4){bottom:0;left:0;-webkit-animation-delay:1.2s;animation-delay:1.2s}.ant-spin-dot-spin{transform:rotate(45deg);-webkit-animation:antRotate 1.2s infinite linear;animation:antRotate 1.2s infinite linear}.ant-spin-sm .ant-spin-dot{font-size:14px}.ant-spin-sm .ant-spin-dot i{width:6px;height:6px}@-webkit-keyframes antSpinMove{to{opacity:1}}@keyframes antSpinMove{to{opacity:1}}@-webkit-keyframes antRotate{to{transform:rotate(405deg)}}@keyframes antRotate{to{transform:rotate(405deg)}}`
  document.head.appendChild(styleEl)

  const App = () => {
    return createElement(
      Fragment,
      null,
      createElement('code', null, '<Spin />'),
      createElement('em', { style: { padding: '0 10px' } }, '→'),
      createElement(Spin, { size: 'small' }),
    )
  }
  render(createElement(App), t.$span)

  t.ok()
})

test(['react@17', 'react-dom@17', '@material-ui/core?deps=react@17,react-dom@17'], async (t) => {
  const [
    { createElement, useState },
    { render },
    { Button },
  ] = t.modules

  const App = () => {
    const [count, setCount] = useState(0)
    return createElement(
      Button,
      { size: 'small', variant: 'outlined', color: 'secondary', onClick: () => setCount(count + 1) },
      `Clicked ${count}`
    )
  }
  render(createElement(App), t.$span)

  t.ok()
})

test(['preact', 'preact/hooks'], async (t) => {
  const [
    { h, render },
    { useState }
  ] = t.modules

  const App = () => {
    const [count, setCount] = useState(0)
    return h('span', {
      onClick: () => setCount(n => n + 1),
      style: { cursor: 'pointer', userSelect: 'none' },
    }, '⏱ ', h('samp', null, count))
  }
  render(h(App), t.$span)

  t.ok()
})

test(['preact@10.7.2', 'preact@10.7.2/hooks', 'swr?alias=react:preact/compat&deps=preact@10.7.2'], async (t) => {
  const [
    { Fragment, h, render },
    { useEffect },
    { default: useSWR }
  ] = t.modules

  const fetcher = (url) => fetch(url).then((res) => res.json());
  const App = () => {
    const { data, error } = useSWR('./status.json', fetcher)
    useEffect(() => {
      t.$span.removeChild(t.$span.lastChild)
    }, [])
    return h(
      Fragment,
      null,
      error && h('span', null, 'failed to load'),
      !data && h('span', null, 'loading...'),
      data && h('span', null, 'server uptime: ', h('strong', null, data.uptime)),
    )
  }
  render(h(App), t.$span)

  t.ok()
})

test('vue@2', async (t) => {
  const { default: Vue } = t.module

  new Vue({
    el: t.$span,
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
          h(
            'span',
            {
              style: { cursor: 'pointer', userSelect: 'none' },
              on: { click: this.onClick },
            },
            ['⏱ ', h('samp', {}, this.count)]
          )
        ]
      )
    }
  })

  t.ok()
})

test('vue@3', async (t) => {
  const { createApp, h } = t.module

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
      return h(
        'span',
        {
          style: { cursor: 'pointer', userSelect: 'none' },
          onClick: this.onClick,
        },
        '⏱ ',
        h('samp', {}, this.count),
      )
    }
  }).mount(t.$span)

  t.ok()
})


test(['rxjs@7', 'rxjs@7/operators'], async (t) => {
  const [{ fromEvent }, { throttleTime, scan }] = t.modules

  t.$span.innerText = '⏱ 0'
  t.$span.style.cursor = 'pointer'
  t.$span.style.userSelect = 'none'
  fromEvent(t.$span, 'click')
    .pipe(
      throttleTime(1000),
      scan(count => count + 1, 0)
    )
    .subscribe(count => t.$span.innerText = `⏱ ${count}`);

  t.ok()
})

test('jquery', async (t) => {
  const { default: $ } = t.module

  $(t.$span).css({ color: 'gray' }).text('$')

  t.ok()
})

test('lodash', async (t) => {
  const { default: _ } = t.module

  const defaults = _.defaults({ lodash: '_' }, { lodash: 'lodash' })
  t.$span.style.color = 'gray'
  t.$span.innerText = defaults.lodash

  t.ok()
})

test('d3', async (t) => {
  const d3 = t.module

  t.$span.id = 'd3-span'
  d3.select('#d3-span').style('color', 'gray').text('d3')

  t.ok()
})

test('pixi.js', async (t) => {
  const { Application, Sprite } = t.module

  const app = new Application({ width: 30, height: 30, resolution: 2, backgroundAlpha: 0 });
  app.loader.add('bunny', 'https://pixijs.io/examples/examples/assets/bunny.png').load((_, resources) => {
    const bunny = new Sprite(resources.bunny.texture);
    bunny.anchor.set(0.5);
    bunny.scale.x = bunny.scale.y = 0.5;
    bunny.x = app.screen.width / 2;
    bunny.y = app.screen.height / 2;

    app.ticker.add(() => {
      bunny.rotation += 0.05;
    });

    app.stage.addChild(bunny);
    t.ok()
  });

  app.view.style.width = '30px'
  app.view.style.height = '30px'
  t.$span.appendChild(app.view);
})

test('three', async (t) => {
  const {
    Scene,
    PerspectiveCamera,
    WebGLRenderer,
    BoxGeometry,
    MeshBasicMaterial,
    Mesh,
  } = t.module

  const width = 30
  const height = 30
  const scene = new Scene();
  const camera = new PerspectiveCamera(75, width / height, 0.1, 1000);
  const renderer = new WebGLRenderer({ alpha: true });
  const geometry = new BoxGeometry();
  const material = new MeshBasicMaterial({ color: 0x000000, wireframe: true });
  const cube = new Mesh(geometry, material);

  renderer.setSize(width, height);
  renderer.setPixelRatio(window.devicePixelRatio);
  camera.position.z = 2;
  scene.add(cube);

  const update = function () {
    requestAnimationFrame(update);

    cube.rotation.x += 0.05;
    cube.rotation.y += 0.05;

    renderer.render(scene, camera);
  };

  update();

  t.$span.appendChild(renderer.domElement);
  t.ok()
})
