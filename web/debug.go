//go:build debug

package web

import (
	"fmt"
	"time"
)

var VERSION = fmt.Sprintf("%x", time.Now().Unix())
var DEBUG = true
