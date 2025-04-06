package server

import (
	"strings"

	"github.com/Masterminds/semver/v3"
	esbuild "github.com/ije/esbuild-internal/api"
)

var targets = map[string]esbuild.Target{
	"es2015":   esbuild.ES2015,
	"es2016":   esbuild.ES2016,
	"es2017":   esbuild.ES2017,
	"es2018":   esbuild.ES2018,
	"es2019":   esbuild.ES2019,
	"es2020":   esbuild.ES2020,
	"es2021":   esbuild.ES2021,
	"es2022":   esbuild.ES2022,
	"es2023":   esbuild.ES2023,
	"es2024":   esbuild.ES2024,
	"esnext":   esbuild.ESNext,
	"deno":     esbuild.ESNext,
	"denonext": esbuild.ESNext,
	"node":     esbuild.ESNext,
}

func getBuildTargetByUA(ua string) string {
	if strings.HasPrefix(ua, "ES/") {
		t := "es" + ua[3:]
		if _, ok := targets[t]; ok {
			return t
		}
	}
	if strings.HasPrefix(ua, "Deno/") {
		version, err := semver.NewVersion(ua[5:])
		// legacy target "deno" (< 1.33.2) doesn't support `node:` specific features
		if err == nil && !(version.Major() > 1 || (version.Major() == 1 && (version.Minor() > 33 || (version.Minor() == 33 && version.Patch() >= 2)))) {
			return "deno"
		}
		return "denonext"
	}
	if ua == "undici" || strings.HasPrefix(ua, "Node.js/") || strings.HasPrefix(ua, "Node/") || strings.HasPrefix(ua, "Bun/") {
		return "node"
	}
	return "es2022"
}
