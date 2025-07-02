package npm

import (
	"testing"
)


func TestResolveVersionByTime(t *testing.T) {
	// Create test metadata with known versions and times
	metadata := &PackageMetadata{
		Time: map[string]string{
			"created":  "2020-01-01T00:00:00Z",
			"modified": "2025-01-01T00:00:00Z",
			"1.0.0":    "2020-06-01T00:00:00Z", // 1590969600
			"1.1.0":    "2021-01-01T00:00:00Z", // 1609459200
			"2.0.0":    "2022-01-01T00:00:00Z", // 1640995200
			"2.1.0":    "2023-01-01T00:00:00Z", // 1672531200
			"3.0.0":    "2024-01-01T00:00:00Z", // 1704067200
		},
		Versions: map[string]PackageJSONRaw{
			"1.0.0": {Version: "1.0.0"},
			"1.1.0": {Version: "1.1.0"},
			"2.0.0": {Version: "2.0.0"},
			"2.1.0": {Version: "2.1.0"},
			"3.0.0": {Version: "3.0.0"},
		},
	}

	tests := []struct {
		name        string
		timestamp   string
		wantVersion string
		wantErr     bool
	}{
		{
			name:        "Before any versions",
			timestamp:   "1577836800s", // 2020-01-01 00:00:00 UTC
			wantVersion: "",
			wantErr:     true,
		},
		{
			name:        "Exact match with version time",
			timestamp:   "1590969600s", // 2020-06-01 00:00:00 UTC (exact time of 1.0.0)
			wantVersion: "1.0.0",
		},
		{
			name:        "Between versions",
			timestamp:   "1620000000s", // 2021-05-02 (between 1.1.0 and 2.0.0)
			wantVersion: "1.1.0",
		},
		{
			name:        "Latest available",
			timestamp:   "1735689600s", // 2025-01-01 00:00:00 UTC (after all versions)
			wantVersion: "3.0.0",
		},
		{
			name:        "Invalid timestamp format",
			timestamp:   "invalid",
			wantVersion: "",
			wantErr:     true,
		},
		{
			name:        "Missing 's' suffix",
			timestamp:   "1620000000",
			wantVersion: "",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveVersionByTime(metadata, tt.timestamp)
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

func TestResolveVersionByTimeWithConstraint(t *testing.T) {
	// Create test metadata with known versions and times
	metadata := &PackageMetadata{
		DistTags: map[string]string{
			"latest": "3.0.0",
			"beta":   "4.0.0-beta.1",
		},
		Time: map[string]string{
			"created":        "2020-01-01T00:00:00Z",
			"modified":       "2025-01-01T00:00:00Z",
			"1.0.0":          "2020-06-01T00:00:00Z", // 1590969600
			"1.1.0":          "2021-01-01T00:00:00Z", // 1609459200
			"1.2.0":          "2021-06-01T00:00:00Z", // 1622505600
			"2.0.0":          "2022-01-01T00:00:00Z", // 1640995200
			"2.1.0":          "2023-01-01T00:00:00Z", // 1672531200
			"3.0.0":          "2024-01-01T00:00:00Z", // 1704067200
			"4.0.0-beta.1":   "2024-06-01T00:00:00Z", // 1717200000
		},
		Versions: map[string]PackageJSONRaw{
			"1.0.0":        {Version: "1.0.0"},
			"1.1.0":        {Version: "1.1.0"},
			"1.2.0":        {Version: "1.2.0"},
			"2.0.0":        {Version: "2.0.0"},
			"2.1.0":        {Version: "2.1.0"},
			"3.0.0":        {Version: "3.0.0"},
			"4.0.0-beta.1": {Version: "4.0.0-beta.1"},
		},
	}

	tests := []struct {
		name              string
		timestamp         string
		versionConstraint string
		wantVersion       string
		wantErr           bool
	}{
		{
			name:              "Latest constraint",
			timestamp:         "1735689600s", // 2025-01-01 (after all versions)
			versionConstraint: "latest",
			wantVersion:       "3.0.0", // latest dist-tag points to 3.0.0
		},
		{
			name:              "Major version constraint",
			timestamp:         "1640995200s", // 2022-01-01 (exact time of 2.0.0)
			versionConstraint: "1",
			wantVersion:       "1.2.0", // Latest 1.x version available at that time
		},
		{
			name:              "Semver range constraint",
			timestamp:         "1672531200s", // 2023-01-01 (exact time of 2.1.0)
			versionConstraint: "^1.0.0",
			wantVersion:       "1.2.0", // Latest 1.x version compatible with ^1.0.0
		},
		{
			name:              "No versions match constraint and time",
			timestamp:         "1577836800s", // 2020-01-01 (before any versions)
			versionConstraint: "1",
			wantVersion:       "",
			wantErr:           true,
		},
		{
			name:              "Future constraint not available at time",
			timestamp:         "1622505600s", // 2021-06-01 (time of 1.2.0)
			versionConstraint: "3",
			wantVersion:       "",
			wantErr:           true,
		},
		{
			name:              "Beta version with prerelease in constraint",
			timestamp:         "1717200000s", // 2024-06-01 (exact time of beta)
			versionConstraint: "4.0.0-beta.1",
			wantVersion:       "4.0.0-beta.1",
		},
		{
			name:              "Beta version excluded by constraint without prerelease",
			timestamp:         "1717200000s", // 2024-06-01 (exact time of beta)
			versionConstraint: "4",
			wantVersion:       "",
			wantErr:           true,
		},
		{
			name:              "Dist tag constraint within time limit",
			timestamp:         "1704067200s", // 2024-01-01 (exact time of 3.0.0)
			versionConstraint: "latest",
			wantVersion:       "3.0.0", // latest is 3.0.0 and was published at this time
		},
		{
			name:              "Dist tag constraint outside time limit",
			timestamp:         "1672531200s", // 2023-01-01 (before latest was published)
			versionConstraint: "latest",
			wantVersion:       "2.1.0", // Falls back to semver, latest available
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveVersionByTimeWithConstraint(metadata, tt.timestamp, tt.versionConstraint)
			if (err != nil) != tt.wantErr {
				t.Errorf("ResolveVersionByTimeWithConstraint() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.wantVersion {
				t.Errorf("ResolveVersionByTimeWithConstraint() = %v, want %v", got, tt.wantVersion)
			}
		})
	}
}

