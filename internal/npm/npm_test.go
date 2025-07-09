package npm

import (
	"testing"
	"time"
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
			name:        "Between versions",
			targetTime:  time.Unix(1620000000, 0), // 2021-05-02 (between 1.1.0 and 2.0.0)
			wantVersion: "1.1.0",
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


