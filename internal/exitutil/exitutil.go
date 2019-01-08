package exitutil

import (
	"os"
	"sync"
)

var (
	mu    sync.Mutex
	funcs []func()
)

func init() {
	funcs = []func(){}
}

func AtExit(f func()) {
	mu.Lock()
	funcs = append([]func(){f}, funcs...)
	mu.Unlock()
}

func Exit(status int) {
	mu.Lock()
	for _, f := range funcs {
		f()
	}
	os.Exit(status)
}
