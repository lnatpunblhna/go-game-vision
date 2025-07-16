package utils

import (
	"fmt"
	"log"
	"os"
)

// LogLevel defines log levels
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
)

// Logger log recorder struct
type Logger struct {
	level  LogLevel
	logger *log.Logger
}

// NewLogger creates a new logger
func NewLogger(level LogLevel) *Logger {
	return &Logger{
		level:  level,
		logger: log.New(os.Stdout, "", log.LstdFlags),
	}
}

// Debug logs debug information
func (l *Logger) Debug(format string, args ...interface{}) {
	if l.level <= DEBUG {
		l.log("DEBUG", format, args...)
	}
}

// Info logs information
func (l *Logger) Info(format string, args ...interface{}) {
	if l.level <= INFO {
		l.log("INFO", format, args...)
	}
}

// Warn logs warnings
func (l *Logger) Warn(format string, args ...interface{}) {
	if l.level <= WARN {
		l.log("WARN", format, args...)
	}
}

// Error logs errors
func (l *Logger) Error(format string, args ...interface{}) {
	if l.level <= ERROR {
		l.log("ERROR", format, args...)
	}
}

// log internal logging method
func (l *Logger) log(level, format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	l.logger.Printf("[%s] %s", level, message)
}

// GlobalLogger global logger instance
var GlobalLogger = NewLogger(INFO)

// Convenience functions
func Debug(format string, args ...interface{}) {
	GlobalLogger.Debug(format, args...)
}

func Info(format string, args ...interface{}) {
	GlobalLogger.Info(format, args...)
}

func Warn(format string, args ...interface{}) {
	GlobalLogger.Warn(format, args...)
}

func Error(format string, args ...interface{}) {
	GlobalLogger.Error(format, args...)
}
