import simpleTest from './simple_test.js';

const assert = {
    hasNamedExport: (mod, name) => {
        if (mod[name] === undefined) throw new Error(`[namedExport] ${name} is not exist`);
    },
    hasDefault: (mod, name) => {
        if (mod[name] === undefined) throw new Error(`[default] ${name} is not exist`);
    },
}

export async function test(ul, match) {
    const _esm = async (name, testFn) => {
        if (!['all', 'export-names'].includes(match) && !(Array.isArray(name) && name.some(n => n.includes(match))) && !name.includes(match)) {
            return
        }
        const li = document.createElement('li')
        const strong = document.createElement('strong')
        const span = document.createElement('span')
        const em = document.createElement('em')
        const names = [name].flat()
        names.forEach((name, i) => {
            const a = document.createElement('a')
            a.innerText = `${Array.isArray(name) ? name[0] : name}`
            a.href = `/${Array.isArray(name) ? name[0] : name}${name.includes('?') ? '&' : '?'}dev`
            strong.appendChild(a)
            if (i < names.length - 1) {
                strong.appendChild(document.createTextNode(', '))
            }
        })
        strong.appendChild(document.createTextNode(':'))
        span.innerText = 'importing...'
        em.style.display = 'none'
        li.appendChild(strong)
        li.appendChild(span)
        li.appendChild(em)
        ul.appendChild(li)
        try {
            const domain = localStorage.importDomain || '';
            const t1 = Date.now()
            const mod = Array.isArray(name) ? await Promise.all(name.map(n => {
                return import(`${domain}/${n}${n.includes('?') ? '&' : '?'}dev`)
            })) : await import(`${domain}/${name}${name.includes('?') ? '&' : '?'}dev`)
            const t2 = Date.now()

            try {
                await testFn({ mod, span })
            } catch (err) {
                span.innerText = `❌ ${err.message}`;
            }

            const t3 = Date.now()
            em.innerText = `· import in ${Math.round(t2 - t1)}ms, run in ${Math.round(t3 - t2)}ms`
            em.style.display = 'inline-block'
        } catch (e) {
            span.innerText = '❌ ' + e.message
        }
    }

    if (match === 'export-names') {
        for (const st of simpleTest) {
            await _esm(st.name, async (t) => {
                t.span.id = st.name;
                if (st.namedExport) {
                    for (let i = 0; i < st.namedExport.length; i++) {
                        assert.hasNamedExport(t.mod, st.namedExport[i])
                    }
                }
                if (st.default) {
                    for (let i = 0; i < st.default.length; i++) {
                        assert.hasDefault(t.mod.default, st.default[i])
                    }
                }
                if (st.defaultIs) {
                    if (typeof t.mod.default !== st.defaultIs) {
                        throw new Error(`default is not ${st.defaultIs}`);
                    }
                }

                t.span.innerText = '✅';
            })
        }
        return
    }

    _esm(['react@16', 'react-dom@16'], async (t) => {
        const [
            { createElement, Fragment, useState },
            { render }
        ] = t.mod
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
            { createElement, Fragment, useState },
            { render }
        ] = t.mod
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

    _esm(['react@17', 'react-dom@17', 'react-redux?deps=react@17', 'redux'], async (t) => {
        const [
            { createElement, Fragment },
            { render },
            { Provider, useDispatch, useSelector },
            { createStore }
        ] = t.mod
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
        ] = t.mod
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

    _esm(['preact', 'preact/hooks'], async (t) => {
        const [
            { Fragment, h, render },
            { useEffect, useState }
        ] = t.mod
        const App = () => {
            const [count, setCount] = useState(0)
            useEffect(() => {
                t.span.removeChild(t.span.lastChild)
            }, [])
            return h(
                Fragment,
                {

                },
                h('span', null, '✅'),
                h('span', {
                    onClick: () => setCount(n => n + 1),
                    style: { cursor: 'pointer', userSelect: 'none' },
                }, ' ⏱ ', h('samp', null, count))
            )
        }
        render(h(App), t.span)
    })

    _esm('vue@2', async (t) => {
        const { default: Vue } = t.mod
        new Vue({
            el: t.span,
            render(h) {
                return h('span', null, '✅')
            }
        })
    })

    _esm('vue@3', async (t) => {
        const { createApp, h } = t.mod
        createApp({
            render() {
                return h('span', {}, '✅')
            }
        }).mount(t.span)
    })

    _esm('jquery', async (t) => {
        const { default: $ } = t.mod
        $(t.span).text('✅')
    })

    _esm('lodash', async (t) => {
        const { default: _ } = t.mod
        const defaults = _.defaults({ ok: '✅' }, { ok: '❌' })
        t.span.innerText = defaults.ok
    })

    _esm('d3', async (t) => {
        const d3 = t.mod
        t.span.id = 'd3-span'
        d3.select('#d3-span').text('✅')
    })

}