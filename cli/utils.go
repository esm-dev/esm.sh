package cli

import (
	"encoding/base64"
	"errors"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
)

// isHttpSepcifier returns true if the specifier is a remote URL.
func isHttpSepcifier(specifier string) bool {
	return strings.HasPrefix(specifier, "https://") || strings.HasPrefix(specifier, "http://")
}

// isRelativeSpecifier returns true if the specifier is a local path.
func isRelativeSpecifier(specifier string) bool {
	return strings.HasPrefix(specifier, "./") || strings.HasPrefix(specifier, "../")
}

func isAbsolutePathSpecifier(specifier string) bool {
	return strings.HasPrefix(specifier, "/") || strings.HasPrefix(specifier, "file://")
}

// checks if the given attribute name is a W3C standard attribute.
func isW3CStandardAttribute(attr string) bool {
	switch attr {
	case "id", "href", "src", "name", "placeholder", "rel", "role", "selected", "checked", "slot", "style", "tilte", "type", "value", "width", "height", "hidden", "dir", "dragable", "lang", "spellcheck", "tabindex", "translate", "popover":
		return true
	default:
		return strings.HasPrefix(attr, "aria-") || strings.HasPrefix(attr, "data-")
	}
}

// btoaUrl converts a string to a base64 string.
func btoaUrl(s string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(s))
}

// atobUrl converts a base64 string to a string.
func atobUrl(s string) (string, error) {
	data, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// bundleModule builds the remote module and it's submodules.
func bundleModule(entry string) ([]byte, error) {
	ret := api.Build(api.BuildOptions{
		EntryPoints:      []string{entry},
		Bundle:           true,
		Format:           api.FormatESModule,
		Target:           api.ESNext,
		Platform:         api.PlatformBrowser,
		MinifyWhitespace: true,
		MinifySyntax:     true,
		JSX:              api.JSXPreserve,
		LegalComments:    api.LegalCommentsNone,
		Plugins: []api.Plugin{
			{
				Name: "external-resolver",
				Setup: func(build api.PluginBuild) {
					build.OnResolve(api.OnResolveOptions{Filter: ".*"}, func(args api.OnResolveArgs) (api.OnResolveResult, error) {
						path := args.Path
						if isHttpSepcifier(args.Path) || (!isRelativeSpecifier(args.Path) && !isAbsolutePathSpecifier(args.Path)) {
							return api.OnResolveResult{Path: path, External: true}, nil
						}
						return api.OnResolveResult{}, nil
					})
				},
			},
		},
	})
	if len(ret.Errors) > 0 {
		return nil, errors.New(ret.Errors[0].Text)
	}
	return ret.OutputFiles[0].Contents, nil
}
