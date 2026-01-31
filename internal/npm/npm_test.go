package npm

import (
	"testing"
	"time"
)

func TestIsExactVersion(t *testing.T) {
	tests := []struct {
		name    string
		version string
	}{
		// exact versions
		{"Simple exact version", "1.0.0"},
		{"exact version with patch", "1.2.3"},
		{"exact version with build metadata", "1.0.0-1"},
		{"exact version with build metadata", "1.0.0+build.1"},

		// Experimental versions
		{"Experimental version", "0.0.0-experimental-c5b937576-20231219"},
		{"Experimental with caps", "1.0.0-EXPERIMENTAL"},
		{"Experimental in middle", "1.0.0-experimental.1"},

		// Beta versions
		{"Beta version", "1.0.0-beta"},
		{"Beta with number", "1.0.0-beta.1"},
		{"Beta with caps", "1.0.0-BETA"},

		// Alpha versions
		{"Alpha version", "1.0.0-alpha"},
		{"Alpha with number", "1.0.0-alpha.1"},

		// RC versions
		{"Release candidate", "1.0.0-rc"},
		{"Release candidate with number", "1.0.0-rc.1"},

		// Other prerelease versions
		{"Preview version", "1.0.0-preview"},
		{"Canary version", "1.0.0-canary"},
		{"Dev version", "1.0.0-dev"},
		{"Nightly version", "1.0.0-nightly"},
		{"Next version", "1.0.0-next"},
		{"Edge version", "1.0.0-edge"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !IsExactVersion(tt.version) {
				t.Errorf("IsExactVersion(%q) = false, want true", tt.version)
			}
		})
	}
}

func TestResolveVersionByTime(t *testing.T) {
	// Create test metadata with known versions and times, including experimental versions
	metadata := &PackageMetadata{
		Time: map[string]string{
			"created":                               "2020-01-01T00:00:00Z",
			"modified":                              "2025-01-01T00:00:00Z",
			"1.0.0":                                 "2020-06-01T00:00:00Z",
			"0.0.0-experimental-c5b937576-20231219": "2020-12-19T00:00:00Z", // Experimental version between 1.0.0 and 1.1.0
			"1.1.0":                                 "2021-01-01T00:00:00Z",
			"1.2.0-beta.1":                          "2021-06-01T00:00:00Z", // Beta version between 1.1.0 and 2.0.0
			"2.0.0":                                 "2022-01-01T00:00:00Z",
			"2.1.0":                                 "2023-01-01T00:00:00Z",
			"3.0.0":                                 "2024-01-01T00:00:00Z",
		},
		Versions: map[string]PackageJSONRaw{
			"1.0.0":                                 {Version: "1.0.0"},
			"0.0.0-experimental-c5b937576-20231219": {Version: "0.0.0-experimental-c5b937576-20231219"},
			"1.1.0":                                 {Version: "1.1.0"},
			"1.2.0-beta.1":                          {Version: "1.2.0-beta.1"},
			"2.0.0":                                 {Version: "2.0.0"},
			"2.1.0":                                 {Version: "2.1.0"},
			"3.0.0":                                 {Version: "3.0.0"},
		},
	}

	tests := []struct {
		name        string
		targetTime  time.Time
		wantVersion string
		wantErr     bool
	}{
		{
			name:        "Before any versions",
			targetTime:  time.Unix(1577836800, 0), // 2020-01-01 00:00:00 UTC
			wantVersion: "",
			wantErr:     true,
		},
		{
			name:        "Exact match with version time",
			targetTime:  time.Unix(1590969600, 0), // 2020-06-01 00:00:00 UTC (exact time of 1.0.0)
			wantVersion: "1.0.0",
		},
		{
			name:        "Skip experimental version, return exact",
			targetTime:  time.Unix(1608336000, 0), // 2020-12-19 00:00:00 UTC (exact time of experimental version)
			wantVersion: "1.0.0",                  // Should return 1.0.0, not the experimental version
		},
		{
			name:        "Between versions, skip experimental",
			targetTime:  time.Unix(1620000000, 0), // 2021-05-02 (between 1.1.0 and 2.0.0, experimental exists but should be ignored)
			wantVersion: "1.1.0",
		},
		{
			name:        "Skip beta version, return exact",
			targetTime:  time.Unix(1622505600, 0), // 2021-06-01 00:00:00 UTC (exact time of beta version)
			wantVersion: "1.1.0",                  // Should return 1.1.0, not the beta version
		},
		{
			name:        "Latest available",
			targetTime:  time.Unix(1735689600, 0), // 2025-01-01 00:00:00 UTC (after all versions)
			wantVersion: "3.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveVersionByTime(metadata, tt.targetTime)
			if (err != nil) != tt.wantErr {
				t.Errorf("ResolveVersionByTime() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.wantVersion {
				t.Errorf("ResolveVersionByTime() = %v, want %v", got, tt.wantVersion)
			}
		})
	}
}
