package server

import (
	"errors"
	"io/ioutil"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/ije/esbuild-internal/compat"
	"github.com/ije/esbuild-internal/js_ast"
	"github.com/ije/esbuild-internal/js_parser"
	"github.com/ije/esbuild-internal/logger"
	"github.com/mssola/user_agent"
)

var regBrowserVersion = regexp.MustCompile(`^([0-9]+)(?:\.([0-9]+))?(?:\.([0-9]+))?$`)

var targets = map[string]api.Target{
	"es2015": api.ES2015,
	"es2016": api.ES2016,
	"es2017": api.ES2017,
	"es2018": api.ES2018,
	"es2019": api.ES2019,
	"es2020": api.ES2020,
	"es2021": api.ES2021,
	"es2022": api.ES2022,
	"esnext": api.ESNext,
	"node":   api.ESNext,
	"deno":   api.ESNext,
}

var engines = map[string]api.EngineName{
	"node":    api.EngineNode,
	"chrome":  api.EngineChrome,
	"edge":    api.EngineEdge,
	"firefox": api.EngineFirefox,
	"ios":     api.EngineIOS,
	"safari":  api.EngineSafari,
	"opera":   api.EngineOpera,
}

var jsFeatures = []compat.JSFeature{
	compat.ArbitraryModuleNamespaceNames,
	compat.ArraySpread,
	compat.Arrow,
	compat.AsyncAwait,
	compat.AsyncGenerator,
	compat.Bigint,
	compat.Class,
	compat.ClassField,
	compat.ClassPrivateAccessor,
	compat.ClassPrivateBrandCheck,
	compat.ClassPrivateField,
	compat.ClassPrivateMethod,
	compat.ClassPrivateStaticAccessor,
	compat.ClassPrivateStaticField,
	compat.ClassPrivateStaticMethod,
	compat.ClassStaticBlocks,
	compat.ClassStaticField,
	compat.ConstAndLet,
	compat.DefaultArgument,
	compat.Destructuring,
	compat.DynamicImport,
	compat.ExponentOperator,
	compat.ExportStarAs,
	compat.ForAwait,
	compat.ForOf,
	compat.Generator,
	compat.Hashbang,
	compat.ImportAssertions,
	compat.ImportMeta,
	compat.LogicalAssignment,
	compat.NestedRestBinding,
	compat.NewTarget,
	compat.NodeColonPrefixImport,
	compat.NodeColonPrefixRequire,
	compat.NullishCoalescing,
	compat.ObjectAccessors,
	compat.ObjectExtensions,
	compat.ObjectRestSpread,
	compat.OptionalCatchBinding,
	compat.OptionalChain,
	compat.RegexpDotAllFlag,
	compat.RegexpLookbehindAssertions,
	compat.RegexpMatchIndices,
	compat.RegexpNamedCaptureGroups,
	compat.RegexpStickyAndUnicodeFlags,
	compat.RegexpUnicodePropertyEscapes,
	compat.RestArgument,
	compat.TemplateLiteral,
	compat.TopLevelAwait,
	compat.TypeofExoticObjectIsObject,
	compat.UnicodeEscapes,
}

func validateESMAFeatures(target api.Target) int {
	constraints := make(map[compat.Engine][]int)

	switch target {
	case api.ES2015:
		constraints[compat.ES] = []int{2015}
	case api.ES2016:
		constraints[compat.ES] = []int{2016}
	case api.ES2017:
		constraints[compat.ES] = []int{2017}
	case api.ES2018:
		constraints[compat.ES] = []int{2018}
	case api.ES2019:
		constraints[compat.ES] = []int{2019}
	case api.ES2020:
		constraints[compat.ES] = []int{2020}
	case api.ES2021:
		constraints[compat.ES] = []int{2021}
	case api.ES2022:
		constraints[compat.ES] = []int{2022}
	case api.ESNext:
	default:
		panic("invalid target")
	}

	return countFeatures(compat.UnsupportedJSFeatures(constraints))
}

func validateEngineFeatures(engine api.Engine) int {
	constraints := make(map[compat.Engine][]int)

	if match := regBrowserVersion.FindStringSubmatch(engine.Version); match != nil {
		if major, err := strconv.Atoi(match[1]); err == nil {
			version := []int{major}
			if minor, err := strconv.Atoi(match[2]); err == nil {
				version = append(version, minor)
			}
			if patch, err := strconv.Atoi(match[3]); err == nil {
				version = append(version, patch)
			}
			switch engine.Name {
			case api.EngineNode:
				constraints[compat.Node] = version
			case api.EngineChrome:
				constraints[compat.Chrome] = version
			case api.EngineEdge:
				constraints[compat.Edge] = version
			case api.EngineFirefox:
				constraints[compat.Firefox] = version
			case api.EngineIOS:
				constraints[compat.IOS] = version
			case api.EngineSafari:
				constraints[compat.Safari] = version
			case api.EngineOpera:
				constraints[compat.Opera] = version
			default:
				panic("invalid engine name")
			}
		}
	}

	return countFeatures(compat.UnsupportedJSFeatures(constraints))
}

func countFeatures(feature compat.JSFeature) int {
	n := 0
	for _, f := range jsFeatures {
		if feature&f != 0 {
			n++
		}
	}
	return n
}

func getTargetByUA(ua string) string {
	if strings.HasPrefix(ua, "Deno/") {
		return "deno"
	}
	if strings.HasPrefix(ua, "Node/") {
		return "node"
	}
	name, version := user_agent.New(ua).Browser()
	if engine, ok := engines[strings.ToLower(name)]; ok {
		a := strings.Split(version, ".")
		if len(a) > 3 {
			version = strings.Join(a[:3], ".")
		}
		unspportEngineFeatures := validateEngineFeatures(api.Engine{
			Name:    engine,
			Version: version,
		})
		for _, target := range []string{
			"es2022",
			"es2021",
			"es2020",
			"es2019",
			"es2018",
			"es2017",
			"es2016",
		} {
			unspportESMAFeatures := validateESMAFeatures(targets[target])
			if unspportEngineFeatures <= unspportESMAFeatures {
				return target
			}
		}
	}
	return "es2015"
}

func parseESModule(wd string, packageName string, moduleSpecifier string) (resolveName string, exportDefault bool, err error) {
	pkgDir := path.Join(wd, "node_modules", packageName)
	resolveName = moduleSpecifier
	switch path.Ext(moduleSpecifier) {
	case ".js", ".jsx", ".ts", ".tsx", ".mjs":
	default:
		resolveName = moduleSpecifier + ".js"
		if !fileExists(path.Join(pkgDir, resolveName)) {
			resolveName = moduleSpecifier + ".mjs"
		}
	}
	if !fileExists(path.Join(pkgDir, resolveName)) && dirExists(path.Join(pkgDir, moduleSpecifier)) {
		resolveName = path.Join(moduleSpecifier, "index.js")
		if !fileExists(path.Join(pkgDir, resolveName)) {
			resolveName = path.Join(moduleSpecifier, "index.mjs")
		}
	}
	filename := path.Join(pkgDir, resolveName)
	switch path.Ext(filename) {
	case ".js", ".jsx", ".ts", ".tsx", ".mjs":
	default:
		filename += ".js"
	}
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return
	}
	log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil)
	ast, pass := js_parser.Parse(log, logger.Source{
		Index:          0,
		KeyPath:        logger.Path{Text: "<stdin>"},
		PrettyPath:     "<stdin>",
		Contents:       string(data),
		IdentifierName: "stdin",
	}, js_parser.Options{})
	if pass {
		esm := ast.ExportsKind == js_ast.ExportsESM
		if !esm {
			err = errors.New("not a module")
			return
		}
		for name := range ast.NamedExports {
			if name == "default" {
				exportDefault = true
				break
			}
		}
	}
	return
}
