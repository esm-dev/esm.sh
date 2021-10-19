package server

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestNodeServices(t *testing.T) {
	testDir := t.TempDir()

	qs := make(chan bool, 1)
	go startNodeServices(qs, testDir, nil)

	t.Cleanup(func() {
		qs <- true
		fmt.Println(t.Name(), "Cleanup")
	})

	time.Sleep(time.Second / 2)

	data := <-invokeNodeService("test", map[string]interface{}{"foo": "bar"})

	var ret map[string]interface{}
	err := json.Unmarshal(data, &ret)
	if err != nil {
		t.Error(err)
	}
	if ret["foo"] != "bar" {
		t.Error("bad return")
	}
}
