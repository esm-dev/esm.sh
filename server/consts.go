package server

// ems.sh version
const VERSION = 53

const (
	pkgCacheTimeout    = 5 * 60 // 5 minutes
	pkgRequstTimeout   = 30     // 30 seconds
	denoStdNodeVersion = "0.113.0"
)

const cssLoaderTpl = `const id = "%s"
const css = %s
if (!document.querySelector("[data-module-url=\"" + id + "\"]")) {
	const el = document.createElement('style')
	el.type = 'text/css'
	el.setAttribute('data-module-url', id)
	el.appendChild(document.createTextNode(css))
	document.head.appendChild(el)
}`
