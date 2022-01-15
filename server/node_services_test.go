package server

import (
	"context"
	"encoding/json"
	"os"
	"path"
	"testing"
	"time"

	"github.com/ije/gox/crypto/rs"
)

func TestNodeServices(t *testing.T) {
	testDir := t.TempDir()

	if os.Getenv("CI") == "true" {
		t.SkipNow()
	}

	go startNodeServices(context.Background(), testDir, nil)

	for i := 0; i < 100; i++ {
		secret := rs.Hex.String(64)
		data := invokeNodeService("test", map[string]interface{}{"secret": secret})

		var ret map[string]interface{}
		err := json.Unmarshal(data, &ret)
		if err != nil {
			t.Error(err)
		}
		if ret["secret"] != secret {
			t.Error("bad return")
		}
	}

	kill(path.Join(testDir, "ns.pid"))
	time.Sleep(100 * time.Millisecond)
}
