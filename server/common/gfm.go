package common

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/yuin/goldmark"
	meta "github.com/yuin/goldmark-meta"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	goldmark_html "github.com/yuin/goldmark/renderer/html"
	"golang.org/x/net/html"
)

var gfm = goldmark.New(
	goldmark.WithParserOptions(
		parser.WithAutoHeadingID(),
	),
	goldmark.WithExtensions(
		extension.GFM,
		meta.New(meta.WithStoresInDocument()),
	),
	goldmark.WithRendererOptions(
		goldmark_html.WithUnsafe(),
	),
)

func RenderMarkdown(md []byte, kind string) (code []byte, err error) {
	var unSafeHtmlBuf bytes.Buffer
	var htmlBuf bytes.Buffer
	var metaDataJS []byte
	context := parser.NewContext()
	err = gfm.Convert(md, &unSafeHtmlBuf, parser.WithContext(context))
	if err != nil {
		return nil, fmt.Errorf("failed to convert markdown to HTML: %v", err)
	}
	metaData := meta.Get(context)
	if len(metaData) > 0 {
		j, err := json.Marshal(metaData)
		if err != nil {
			metaDataJS = []byte("console.warn('Failed to serialize metadata');export const meta = {};")
		} else {
			metaDataJS = []byte("export const meta = " + string(j) + ";")
		}
	} else {
		metaDataJS = []byte("export const meta = {};")
	}
	tokenizer := html.NewTokenizer(&unSafeHtmlBuf)
	var skipTag []byte
	var skipTagDepth int
	for {
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			if tokenizer.Err() != io.EOF {
				return nil, fmt.Errorf("failed to transform markdown to %s: %v", kind, tokenizer.Err())
			}
			break
		}
		if skipTag != nil {
			if tt == html.StartTagToken {
				tagName, _ := tokenizer.TagName()
				if bytes.Equal(tagName, skipTag) {
					skipTagDepth++
				}
			} else if tt == html.EndTagToken {
				tagName, _ := tokenizer.TagName()
				if bytes.Equal(tagName, skipTag) {
					skipTagDepth--
					if skipTagDepth == 0 {
						skipTag = nil
					}
				}
			}
			continue
		}
		if tt == html.StartTagToken || tt == html.SelfClosingTagToken || tt == html.EndTagToken {
			tagName, moreAttr := tokenizer.TagName()
			switch string(tagName) {
			case "a", "h1", "h2", "h3", "h4", "h5", "h6", "p", "br", "hr", "img", "strong", "em", "del", "sub", "sup", "code", "pre", "blockquote", "ol", "ul", "li", "table", "thead", "tbody", "tfoot", "tr", "th", "td", "caption", "details", "summary", "figure", "figcaption", "audio", "video", "source", "track":
				htmlBuf.WriteByte('<')
				if tt == html.EndTagToken {
					htmlBuf.WriteByte('/')
					htmlBuf.Write(tagName)
					htmlBuf.WriteByte('>')
				} else {
					htmlBuf.Write(tagName)
					for moreAttr {
						var key, val []byte
						key, val, moreAttr = tokenizer.TagAttr()
						switch string(key) {
						case "id", "class", "href", "target", "src", "width", "height", "alt", "align", "border", "title", "dir", "hidden", "role", "lang", "loading", "referrerpolicy", "sizes", "srcset":
							htmlBuf.WriteByte(' ')
							htmlBuf.Write(key)
							if len(val) > 0 {
								htmlBuf.WriteByte('=')
								htmlBuf.WriteByte('"')
								htmlBuf.Write(val)
								htmlBuf.WriteByte('"')
							}
						}
					}
					isSelfClosing := tt == html.SelfClosingTagToken
					if !isSelfClosing {
						switch string(tagName) {
						case "br", "hr", "img":
							isSelfClosing = true
						}
					}
					if isSelfClosing {
						htmlBuf.Write([]byte{'/', '>'})
					} else {
						htmlBuf.WriteByte('>')
					}
				}
			default:
				htmlBuf.Write([]byte("<!-- raw HTML omitted -->"))
				if tt == html.StartTagToken {
					skipTag = tagName
					skipTagDepth = 1
				}
			}
		} else if tt == html.TextToken {
			for _, b := range tokenizer.Text() {
				switch b {
				case '{':
					htmlBuf.WriteString("&lbrace;")
				case '}':
					htmlBuf.WriteString("&rbrace;")
				default:
					htmlBuf.WriteByte(b)
				}
			}
		}
	}
	switch kind {
	case "jsx":
		jsxBuf := bytes.NewBuffer(metaDataJS)
		jsxBuf.Write([]byte("export default function Markdown() { return <>"))
		htmlBuf.WriteTo(jsxBuf)
		jsxBuf.Write([]byte("</>}"))
		return jsxBuf.Bytes(), nil
	case "svelte":
		htmlBuf.Write([]byte("<script module>"))
		htmlBuf.Write(metaDataJS)
		htmlBuf.Write([]byte("</script>"))
		return htmlBuf.Bytes(), nil
	case "vue":
		vueBuf := bytes.NewBuffer([]byte("<script>"))
		vueBuf.Write(metaDataJS)
		vueBuf.Write([]byte("</script><template>"))
		htmlBuf.WriteTo(vueBuf)
		vueBuf.Write([]byte("</template>"))
		return vueBuf.Bytes(), nil
	case "js":
		jsBuf := bytes.NewBuffer([]byte("export const html = "))
		json.NewEncoder(jsBuf).Encode(htmlBuf.String())
		jsBuf.Write([]byte{';'})
		jsBuf.Write(metaDataJS)
		jsBuf.Write([]byte("export default html;"))
		return jsBuf.Bytes(), nil
	default:
		return htmlBuf.Bytes(), nil
	}
}
