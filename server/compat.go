package server

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/evanw/esbuild/pkg/api"
	"github.com/ije/esbuild-internal/compat"
	"github.com/mileusna/useragent"
)

var regexpBrowserVersion = regexp.MustCompile(`^(\d+)(?:\.(\d+))?(?:\.(\d+))?$`)
var v1_33_2 = semver.MustParse("1.33.2")

var targets = map[string]api.Target{
	"es2015":   api.ES2015,
	"es2016":   api.ES2016,
	"es2017":   api.ES2017,
	"es2018":   api.ES2018,
	"es2019":   api.ES2019,
	"es2020":   api.ES2020,
	"es2021":   api.ES2021,
	"es2022":   api.ES2022,
	"esnext":   api.ESNext,
	"deno":     api.ESNext,
	"denonext": api.ESNext,
	"node":     api.ESNext,
}

var browsers = map[string]api.EngineName{
	"chrome":  api.EngineChrome,
	"edge":    api.EngineEdge,
	"firefox": api.EngineFirefox,
	"ios":     api.EngineIOS,
	"opera":   api.EngineOpera,
	"safari":  api.EngineSafari,
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
	compat.Decorators,
	compat.DefaultArgument,
	compat.Destructuring,
	compat.DynamicImport,
	compat.ExponentOperator,
	compat.ExportStarAs,
	compat.ForAwait,
	compat.ForOf,
	compat.FunctionNameConfigurable,
	compat.FunctionOrClassPropertyAccess,
	compat.Generator,
	compat.Hashbang,
	compat.ImportAssertions,
	compat.ImportAttributes,
	compat.ImportMeta,
	compat.InlineScript,
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
	compat.RegexpSetNotation,
	compat.RegexpStickyAndUnicodeFlags,
	compat.RegexpUnicodePropertyEscapes,
	compat.RestArgument,
	compat.TemplateLiteral,
	compat.TopLevelAwait,
	compat.TypeofExoticObjectIsObject,
	compat.UnicodeEscapes,
	compat.Using,
}

func validateESMAFeatures(target api.Target) int {
	constraints := make(map[compat.Engine]compat.Semver)

	switch target {
	case api.ES2015:
		constraints[compat.ES] = compat.Semver{Parts: []int{2015}}
	case api.ES2016:
		constraints[compat.ES] = compat.Semver{Parts: []int{2016}}
	case api.ES2017:
		constraints[compat.ES] = compat.Semver{Parts: []int{2017}}
	case api.ES2018:
		constraints[compat.ES] = compat.Semver{Parts: []int{2018}}
	case api.ES2019:
		constraints[compat.ES] = compat.Semver{Parts: []int{2019}}
	case api.ES2020:
		constraints[compat.ES] = compat.Semver{Parts: []int{2020}}
	case api.ES2021:
		constraints[compat.ES] = compat.Semver{Parts: []int{2021}}
	case api.ES2022:
		constraints[compat.ES] = compat.Semver{Parts: []int{2022}}
	case api.ESNext:
	default:
		panic("invalid target")
	}

	return countFeatures(compat.UnsupportedJSFeatures(constraints))
}

func validateEngineFeatures(engine api.Engine) int {
	constraints := make(map[compat.Engine]compat.Semver)

	if match := regexpBrowserVersion.FindStringSubmatch(engine.Version); match != nil {
		if major, err := strconv.Atoi(match[1]); err == nil {
			version := compat.Semver{Parts: []int{major}}
			if minor, err := strconv.Atoi(match[2]); err == nil {
				version.Parts = append(version.Parts, minor)
			}
			if patch, err := strconv.Atoi(match[3]); err == nil {
				version.Parts = append(version.Parts, patch)
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

func getBrowserInfo(ua string) (name string, version string) {
	browser := useragent.Parse(ua)
	name = browser.Name
	version = browser.Version
	if name == "Headless Chrome" {
		name = "Chrome"
	} else if browser.IsIOS() {
		name = "iOS"
	}
	return
}

func getBuildTargetByUA(ua string) string {
	if ua == "" || strings.HasPrefix(ua, "curl/") {
		return "esnext"
	}
	if strings.HasPrefix(ua, "ES/") {
		t := "es" + ua[3:]
		if _, ok := targets[t]; ok {
			return t
		}
		return "esnext"
	}
	if strings.HasPrefix(ua, "Deno/") {
		uaVersion, err := semver.NewVersion(ua[5:])
		if err == nil && uaVersion.LessThan(v1_33_2) {
			return "deno"
		}
		return "denonext"
	}
	if ua == "undici" || strings.HasPrefix(ua, "Node.js/") || strings.HasPrefix(ua, "Node/") || strings.HasPrefix(ua, "Bun/") {
		return "node"
	}
	name, version := getBrowserInfo(ua)
	if name == "" || version == "" {
		return "esnext"
	}
	if engine, ok := browsers[strings.ToLower(name)]; ok {
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
			"es2015",
		} {
			unspportESMAFeatures := validateESMAFeatures(targets[target])
			if unspportEngineFeatures <= unspportESMAFeatures {
				return target
			}
		}
	}
	return "es2015"
}
