package server

import (
	"encoding/json"
	"testing"
)

func TestTrustedProxiesConfig(t *testing.T) {
	var c Config
	if err := json.Unmarshal([]byte(`{"trustedProxies":["127.0.0.1/32","::1/128"]}`), &c); err != nil {
		t.Fatal(err)
	}
	if len(c.TrustedProxies) != 2 || c.TrustedProxies[0].String() != "127.0.0.1/32" || c.TrustedProxies[1].String() != "::1/128" {
		t.Fatalf("unexpected trusted proxies: %v", c.TrustedProxies)
	}
	if err := json.Unmarshal([]byte(`{"trustedProxies":["invalid"]}`), &c); err == nil {
		t.Fatal("expected invalid proxy CIDR to be rejected")
	}
}

func TestPurgeAPIEnable(t *testing.T) {
	for _, test := range []struct {
		name    string
		input   string
		env     string
		enabled bool
	}{
		{"default", `{}`, "", true},
		{"empty", `{"purgeAPI":{}}`, "", true},
		{"enabled", `{"purgeAPI":{"enable":true}}`, "", true},
		{"disabled", `{"purgeAPI":{"enable":false}}`, "", false},
		{"env disables default", `{}`, "false", false},
		{"env disables explicit enable", `{"purgeAPI":{"enable":true}}`, "false", false},
		{"config stays disabled", `{"purgeAPI":{"enable":false}}`, "true", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("PURGE_CACHE", test.env)
			var c Config
			if err := json.Unmarshal([]byte(test.input), &c); err != nil {
				t.Fatal(err)
			}
			normalizeConfig(&c)
			if c.PurgeAPI.Enable != test.enabled {
				t.Fatalf("PurgeAPI.Enable = %v, want %v", c.PurgeAPI.Enable, test.enabled)
			}
		})
	}
}

func TestPurgeAPICredentials(t *testing.T) {
	t.Setenv("PURGE_GITHUB_CLIENT_ID", "env-client")
	t.Setenv("PURGE_GITHUB_CLIENT_SECRET", "env-secret")
	t.Setenv("PURGE_CLOUDFLARE_ZONE_ID", "env-zone")
	t.Setenv("PURGE_CLOUDFLARE_API_TOKEN", "env-token")
	for _, test := range []struct {
		name  string
		input string
		want  [4]string
	}{
		{"env", `{}`, [4]string{"env-client", "env-secret", "env-zone", "env-token"}},
		{"config", `{"purgeAPI":{"githubClientId":"client","githubClientSecret":"secret","cloudflareZoneId":"zone","cloudflareApiToken":"token"}}`, [4]string{"client", "secret", "zone", "token"}},
		{"mixed", `{"purgeAPI":{"githubClientId":"client","githubClientSecret":"","cloudflareZoneId":"zone","cloudflareApiToken":""}}`, [4]string{"client", "env-secret", "zone", "env-token"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var c Config
			if err := json.Unmarshal([]byte(test.input), &c); err != nil {
				t.Fatal(err)
			}
			normalizeConfig(&c)
			got := [4]string{c.PurgeAPI.GithubClientID, c.PurgeAPI.GithubClientSecret, c.PurgeAPI.CloudflareZoneID, c.PurgeAPI.CloudflareAPIToken}
			if got != test.want {
				t.Fatalf("purge credentials = %v, want %v", got, test.want)
			}
		})
	}
}

func TestNpmQueryCacheTTL(t *testing.T) {
	for _, test := range []struct {
		env      string
		setting  uint32
		expected uint32
	}{
		{"", 0, 600},
		{"30", 0, 30},
		{"0", 0, 0},
		{"invalid", 0, 600},
		{"-1", 0, 600},
		{"4294967296", 0, 600},
		{"30", 90, 90},
	} {
		t.Run(test.env, func(t *testing.T) {
			t.Setenv("NPM_QUERY_CACHE_TTL", test.env)
			c := &Config{NpmQueryCacheTTL: test.setting}
			normalizeConfig(c)
			if c.NpmQueryCacheTTL != test.expected {
				t.Fatalf("NpmQueryCacheTTL = %d, want %d", c.NpmQueryCacheTTL, test.expected)
			}
		})
	}
}

func TestExtractPackageName(t *testing.T) {
	type want struct {
		packageId string
		scope     string
		name      string
		version   string
	}
	tests := []struct {
		name        string
		packageName string
		want        want
	}{
		{
			name:        "PackageWithVersionAndNoScope",
			packageName: "faker@1.5.0",
			want:        want{packageId: "faker@1.5.0", scope: "", name: "faker", version: "1.5.0"},
		},
		{
			name:        "PackageWithVersionAndScope",
			packageName: "@github/faker@1.5.0",
			want:        want{packageId: "@github/faker@1.5.0", scope: "@github", name: "faker", version: "1.5.0"},
		},
		{
			name:        "ReactLoadedFromStable",
			packageName: "react@18.2.0/es2022/react.mjs",
			want:        want{packageId: "react@18.2.0", scope: "", name: "react", version: "18.2.0"},
		},
		{
			name:        "ScopedLoadedFromStable",
			packageName: "@github/faker@0.0.1/es2022/faker.mjs",
			want:        want{packageId: "@github/faker@0.0.1", scope: "@github", name: "faker", version: "0.0.1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fullNameWithoutVersion, scope, name, version := extractPackageName(tt.packageName)

			if fullNameWithoutVersion != tt.want.packageId {
				t.Errorf("%s not equal %s", fullNameWithoutVersion, tt.want.packageId)
			}
			if scope != tt.want.scope {
				t.Errorf("%s not equal %s", scope, tt.want.scope)
			}
			if name != tt.want.name {
				t.Errorf("%s not equal %s", name, tt.want.name)
			}
			if version != tt.want.version {
				t.Errorf("%s not equal %s", version, tt.want.version)
			}
		})
	}
}

func TestAllowListAndBanList_IsPackageNotAllowedOrBanned(t *testing.T) {
	type args struct {
		fullName string
	}
	tests := []struct {
		name      string
		allowList AllowList
		banList   BanList
		args      args
		want      bool
	}{
		{
			name:      "NoAllowOrBanListAllowAnything",
			allowList: AllowList{},
			banList:   BanList{},
			args:      args{fullName: "faker@1.5.0"},
			want:      false,
		},
		{
			name: "AllowedScopeBannedScope",
			allowList: AllowList{
				Scopes: []string{"@github"},
			},
			banList: BanList{
				Scopes: []BanScope{{
					Name: "@github",
				}},
			},
			args: args{fullName: "@github/faker"},
			want: true,
		},
		{
			name: "AllowedScopeBannedPackage",
			allowList: AllowList{
				Scopes: []string{"@github"},
			},
			banList: BanList{
				Packages: []string{"@github/faker"},
			},
			args: args{fullName: "@github/faker"},
			want: true,
		},
		{
			name: "AllowedPackageBannedPackage",
			allowList: AllowList{
				Packages: []string{"@github/faker"},
			},
			banList: BanList{
				Packages: []string{"faker"},
			},
			args: args{fullName: "faker"},
			want: true,
		},
		{
			name: "AllowedPackageBannedScope",
			allowList: AllowList{
				Packages: []string{"faker"},
			},
			banList: BanList{
				Scopes: []BanScope{{
					Name: "@github",
				}},
			},
			args: args{fullName: "@github/faker"},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// to simulate:
			// if !pkgAllowed || pkgBanned {
			//   return rex.Status(403, "forbidden")
			// }
			packageName := tt.args.fullName

			isAllowed := tt.allowList.IsPackageAllowed(packageName)
			isBanned := tt.banList.IsPackageBanned(packageName)

			if got := !isAllowed || isBanned; got != tt.want {
				t.Errorf("isPackageNotAllowedOrBanned() = %v, want %v. %v isAllowed %v, %v isBanned %v", got, tt.want, packageName, isAllowed, packageName, isBanned)
			}
		})
	}
}

func TestAllowList_IsPackageAllowed(t *testing.T) {
	type args struct {
		fullName string
	}
	tests := []struct {
		name      string
		allowList AllowList
		args      args
		want      bool
	}{
		{
			name:      "NoAllowListAllowAnything",
			allowList: AllowList{},
			args:      args{fullName: "faker@1.5.0"},
			want:      true,
		},
		{
			name: "AllowedByPackages",
			allowList: AllowList{
				Packages: []string{"faker"},
			},
			args: args{fullName: "faker"},
			want: true,
		},
		{
			name: "NotAllowedByPackages",
			allowList: AllowList{
				Packages: []string{"allowedPackageName"},
			},
			args: args{fullName: "faker"},
			want: false,
		},
		{
			name: "AllowedByScope",
			allowList: AllowList{
				Scopes: []string{"@github"},
			},
			args: args{fullName: "@github/perfect"},
			want: true,
		},
		{
			name: "NotAllowedByScope",
			allowList: AllowList{
				Scopes: []string{"@github"},
			},
			args: args{fullName: "@faker/perfect"},
			want: false,
		},
		{
			name: "NotAllowedByScope",
			allowList: AllowList{
				Scopes: []string{"@github"},
			},
			args: args{fullName: "@faker/perfect"},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.allowList.IsPackageAllowed(tt.args.fullName); got != tt.want {
				t.Errorf("IsPackageAllowed() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBanList_IsPackageBanned(t *testing.T) {
	type args struct {
		fullName string
	}
	tests := []struct {
		name    string
		banList BanList
		args    args
		want    bool
	}{
		{
			name:    "NotBanned",
			banList: BanList{},
			args:    args{fullName: "faker@1.5.0"},
			want:    false,
		},
		{
			name: "BannedByPackages",
			banList: BanList{
				Packages: []string{"faker"},
			},
			args: args{fullName: "faker"},
			want: true,
		},
		{
			name: "BannedByScopes",
			banList: BanList{
				Scopes: []BanScope{{
					Name:     "@github",
					Excludes: []string{"perfect"},
				}},
			},
			args: args{fullName: "@github/faker@1.0.0"},
			want: true,
		},
		{
			name: "BannedByScopesButExcluded",
			banList: BanList{
				Scopes: []BanScope{{
					Name:     "@github",
					Excludes: []string{"faker"},
				}},
			},
			args: args{fullName: "@github/faker@1.0.0"},
			want: false,
		},
		{
			name: "ExcludedInScopeButBannedByPackages",
			banList: BanList{
				Packages: []string{"@github/faker"},
				Scopes: []BanScope{{
					Name:     "@github",
					Excludes: []string{"faker"},
				}},
			},
			args: args{fullName: "@github/faker@1.0.0"},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.banList.IsPackageBanned(tt.args.fullName); got != tt.want {
				t.Errorf("IsPackageBanned() = %v, want %v", got, tt.want)
			}
		})
	}
}
