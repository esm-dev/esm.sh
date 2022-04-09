package server

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/ije/esbuild-internal/compat"
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
	compat.BigInt,
	compat.Class,
	compat.ClassField,
	compat.ClassPrivateAccessor,
	compat.ClassPrivateBrandCheck,
	compat.ClassPrivateField,
	compat.ClassPrivateMethod,
	compat.ClassPrivateStaticAccessor,
	compat.ClassPrivateStaticField,
	compat.ClassPrivateStaticMethod,
	compat.ClassStaticField,
	compat.Const,
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
	compat.Let,
	compat.LogicalAssignment,
	compat.NestedRestBinding,
	compat.NewTarget,
	compat.NullishCoalescing,
	compat.ObjectAccessors,
	compat.ObjectExtensions,
	compat.ObjectRestSpread,
	compat.OptionalCatchBinding,
	compat.OptionalChain,
	compat.RestArgument,
	compat.TemplateLiteral,
	compat.TopLevelAwait,
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
