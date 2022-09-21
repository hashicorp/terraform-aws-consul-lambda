package trace

import (
	"fmt"

	"github.com/hashicorp/go-hclog"
)

// HCLog is a Logger that uses the go-hclog log package to output timer information.
type HCLog struct {
	// logger is the Logger to use to write the timer information.
	logger hclog.Logger
	// level is the log level at which to output the timer data.
	// The default level is Info if a level is not provided.
	level hclog.Level
}

func NewHCLog(logger hclog.Logger, level hclog.Level) HCLog {
	l := HCLog{level: level}

	if logger != nil {
		l.logger = logger
	} else {
		l.logger = hclog.NewNullLogger()
	}

	return l
}

// Print writes timer information using the logger's Log function, with the configured level.
func (l HCLog) Print(args ...interface{}) {
	l.logger.Log(l.level, fmt.Sprint(args...))
}
