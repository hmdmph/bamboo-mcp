package logger

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"sync"
	"time"
)

var (
	verbose bool
	logger  *log.Logger

	// redactors is the ordered list of patterns that must never appear in logs.
	redactMu       sync.RWMutex
	redactPatterns []*regexp.Regexp
)

func init() {
	logger = log.New(os.Stderr, "", 0)
	// Default: redact Bearer tokens and generic secret-looking values.
	addRedactPattern(`(?i)Bearer\s+[A-Za-z0-9\-._~+/]+=*`)
	addRedactPattern(`(?i)(token|secret|password|api.?key)\s*[:=]\s*\S+`)
}

func SetVerbose(v bool) {
	verbose = v
}

func IsVerbose() bool {
	return verbose
}

// RegisterRedactPattern adds a regex whose matches are replaced with [REDACTED] in all log output.
func RegisterRedactPattern(pattern string) {
	addRedactPattern(pattern)
}

func addRedactPattern(pattern string) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return
	}
	redactMu.Lock()
	redactPatterns = append(redactPatterns, re)
	redactMu.Unlock()
}

// redact replaces all sensitive values in msg before it reaches the log sink.
func redact(msg string) string {
	redactMu.RLock()
	defer redactMu.RUnlock()
	for _, re := range redactPatterns {
		msg = re.ReplaceAllString(msg, "[REDACTED]")
	}
	return msg
}

func Info(format string, args ...interface{}) {
	if !verbose {
		return
	}
	msg := redact(fmt.Sprintf(format, args...))
	logger.Printf("[%s] INFO  %s", time.Now().Format("15:04:05"), msg)
}

func Error(format string, args ...interface{}) {
	if !verbose {
		return
	}
	msg := redact(fmt.Sprintf(format, args...))
	logger.Printf("[%s] ERROR %s", time.Now().Format("15:04:05"), msg)
}

func Debug(format string, args ...interface{}) {
	if !verbose {
		return
	}
	msg := redact(fmt.Sprintf(format, args...))
	logger.Printf("[%s] DEBUG %s", time.Now().Format("15:04:05"), msg)
}
