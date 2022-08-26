package trace

import (
	"fmt"
	"log"

	"github.com/hashicorp/go-hclog"
)

// StdLog is a Logger that uses the stdlib log package to output timer information.
type StdLog struct{}

// Print writes timer information using log.Print.
func (l StdLog) Print(args ...interface{}) {
	log.Print(fmt.Sprint(args...))
}

// HCLog is a Logger that uses the go-hclog log package to output timer information.
type HCLog struct {
	// Logger is the Logger to use to write the timer information.
	Logger hclog.Logger
	// Level is the log level at which to output the timer data.
	// The default level is Info if a level is not provided.
	Level hclog.Level
}

// Print writes timer information using the logger's Log function, with the configured level.
func (l HCLog) Print(args ...interface{}) {
	if l.Logger == nil {
		return
	}
	if l.Level == hclog.NoLevel {
		l.Level = hclog.Info
	}
	l.Logger.Log(l.Level, fmt.Sprint(args...))
}
