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
	SILENT // 静默模式，不输出任何日志
)

var (
	currentLogLevel = INFO
	enableLogging   = true
)

// SetLogLevel sets the global log level
func SetLogLevel(level LogLevel) {
	currentLogLevel = level
}

// SetLoggingEnabled enables or disables logging
func SetLoggingEnabled(enabled bool) {
	enableLogging = enabled
}

// logOutput outputs log message if logging is enabled and level is appropriate
func logOutput(level LogLevel, levelName, format string, args ...interface{}) {
	if !enableLogging || level < currentLogLevel {
		return
	}

	message := fmt.Sprintf(format, args...)
	log.Printf("[%s] %s", levelName, message)
}

// Debug logs debug information
func Debug(format string, args ...interface{}) {
	logOutput(DEBUG, "DEBUG", format, args...)
}

// Info logs information
func Info(format string, args ...interface{}) {
	logOutput(INFO, "INFO", format, args...)
}

// Warn logs warnings
func Warn(format string, args ...interface{}) {
	logOutput(WARN, "WARN", format, args...)
}

// Error logs errors
func Error(format string, args ...interface{}) {
	logOutput(ERROR, "ERROR", format, args...)
}

// init initializes logger with stderr output
func init() {
	log.SetOutput(os.Stderr)
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}
