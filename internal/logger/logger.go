package logger

import (
	"fmt"
	"log"
	"os"
	"strings"
)

// LogLevel represents logging verbosity
type LogLevel int

const (
	// LevelSilent - no logging
	LevelSilent LogLevel = iota
	// LevelError - only errors
	LevelError
	// LevelInfo - normal operation messages
	LevelInfo
	// LevelVerbose - detailed operation messages
	LevelVerbose
	// LevelDebug - everything including debug info
	LevelDebug
)

var (
	currentLevel = LevelInfo
	debugLogger  = log.New(os.Stderr, "[DEBUG] ", log.Ltime|log.Lshortfile)
	verboseLogger = log.New(os.Stderr, "[VERBOSE] ", log.Ltime)
	infoLogger    = log.New(os.Stderr, "[INFO] ", log.Ltime)
	errorLogger   = log.New(os.Stderr, "[ERROR] ", log.Ltime)
)

// Init initializes the logger with the specified level
func Init(level LogLevel) {
	currentLevel = level
}

// InitFromEnv initializes the logger from environment variables
// ROG_VERBOSE: if set to "true" or "1", enables verbose logging
// ROG_DEBUG: if set to "true" or "1", enables debug logging (overrides verbose)
func InitFromEnv() {
	debugEnv := strings.ToLower(os.Getenv("ROG_DEBUG"))
	verboseEnv := strings.ToLower(os.Getenv("ROG_VERBOSE"))

	if debugEnv == "true" || debugEnv == "1" {
		currentLevel = LevelDebug
	} else if verboseEnv == "true" || verboseEnv == "1" {
		currentLevel = LevelVerbose
	}
}

// Debug logs a debug message
func Debug(format string, args ...interface{}) {
	if currentLevel >= LevelDebug {
		debugLogger.Output(2, fmt.Sprintf(format, args...))
	}
}

// Verbose logs a verbose message
func Verbose(format string, args ...interface{}) {
	if currentLevel >= LevelVerbose {
		verboseLogger.Output(2, fmt.Sprintf(format, args...))
	}
}

// Info logs an info message
func Info(format string, args ...interface{}) {
	if currentLevel >= LevelInfo {
		infoLogger.Output(2, fmt.Sprintf(format, args...))
	}
}

// Error logs an error message
func Error(format string, args ...interface{}) {
	if currentLevel >= LevelError {
		errorLogger.Output(2, fmt.Sprintf(format, args...))
	}
}

// GetLevel returns the current log level
func GetLevel() LogLevel {
	return currentLevel
}

// SetLevel sets the current log level
func SetLevel(level LogLevel) {
	currentLevel = level
}
