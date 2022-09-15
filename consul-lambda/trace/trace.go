// Package trace provides simple interfaces for timing applications
package trace

import (
	"runtime"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
)

type Logger interface {
	Print(args ...interface{})
}

var (
	// enabled controls whether or not tracing is enabled globally. By default it is disabled
	// and all API calls simply return without performing any processing to minimize any
	// performance impact from this package.
	enabled bool = false
	// logger is the logger to use to when logging messages.
	// By default timers will use a null logger.
	logger Logger = NewHCLog(nil, hclog.NoLevel)
	// tag is prepended to every trace log message.
	tag = "trace"

	traceMap = make(map[string]*Timer)
	mu       sync.Mutex
)

func Enabled(e bool) {
	if e == enabled {
		return
	}

	mu.Lock()
	defer mu.Unlock()

	enabled = e

	// Purge any existing timers
	for k := range traceMap {
		delete(traceMap, k)
	}
}

func IsEnabled() bool {
	return enabled
}

func Enter() {
	if !enabled {
		return
	}

	mu.Lock()
	defer mu.Unlock()

	// Get calling function
	fname := getCaller()
	if fname == "" {
		return
	}

	// Create a timer and add it to the map
	// TODO: support multiple timers with the same name
	if _, exists := traceMap[fname]; exists {
		return
	}
	traceMap[fname] = Start(fname)
}

func Exit() {
	if !enabled {
		return
	}

	mu.Lock()
	defer mu.Unlock()
	// Get calling function
	fname := getCaller()
	if fname == "" {
		return
	}

	// Get the timer, log the time and remove it from the map
	// TODO: support multiple timers with the same name
	timer, exists := traceMap[fname]
	if !exists {
		return
	}
	timer.Since()
	delete(traceMap, fname)
}

func GetLogger() Logger {
	mu.Lock()
	defer mu.Unlock()
	return logger
}

func SetLogger(l Logger) {
	mu.Lock()
	defer mu.Unlock()
	logger = l
}

func GetTag() string {
	mu.Lock()
	defer mu.Unlock()
	return tag
}

func SetTag(t string) {
	mu.Lock()
	defer mu.Unlock()
	tag = t
}

type Timer struct {
	Tag  string
	Log  Logger
	name string
	t0   time.Time
}

// Start a timer with the specified name.
func Start(name string) *Timer {
	return &Timer{Tag: tag, Log: logger, name: name, t0: time.Now()}
}

// Since logs the time since the timer started. If present the optional args will be appended to the message.
func (t *Timer) Since(args ...interface{}) {
	if t.Log == nil {
		t.Log = NewHCLog(nil, hclog.NoLevel)
	}
	var msg []interface{}
	if tag != "" {
		msg = []interface{}{tag, " ", t.name, ": ", time.Since(t.t0)}
	} else {
		msg = []interface{}{t.name, ": ", time.Since(t.t0)}
	}
	if len(args) > 0 {
		msg = append(msg, " ")
		msg = append(msg, args...)
	}
	t.Log.Print(msg...)
}

func getCaller() string {
	pc := make([]uintptr, 10)
	n := runtime.Callers(3, pc)
	if n == 0 {
		return ""
	}
	pc = pc[:n]
	frames := runtime.CallersFrames(pc)
	if frames == nil {
		return ""
	}
	frame, _ := frames.Next()
	return frame.Function
}
