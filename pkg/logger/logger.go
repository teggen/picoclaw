package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// LogLevel mirrors slog.Level so callers don't need to import slog.
type LogLevel = slog.Level

const (
	DEBUG = slog.LevelDebug
	INFO  = slog.LevelInfo
	WARN  = slog.LevelWarn
	ERROR = slog.LevelError
	FATAL = slog.Level(12) // slog has no FATAL; define above ERROR
)

var (
	logLevelNames = map[LogLevel]string{
		DEBUG: "DEBUG",
		INFO:  "INFO",
		WARN:  "WARN",
		ERROR: "ERROR",
		FATAL: "FATAL",
	}

	currentLevel slog.LevelVar
	consoleLevel slog.LevelVar
	fileLevel    slog.LevelVar

	consoleSlog *slog.Logger
	fileSlog    *slog.Logger
	logFile     *os.File
	mu          sync.RWMutex
)

func init() {
	currentLevel.Set(INFO)
	consoleLevel.Set(INFO)

	consoleSlog = slog.New(newColorHandler(os.Stdout, &consoleLevel))
	fileSlog = nil
}

func SetLevel(level LogLevel) {
	mu.Lock()
	defer mu.Unlock()
	currentLevel.Set(level)
}

func SetConsoleLevel(level LogLevel) {
	mu.Lock()
	defer mu.Unlock()
	consoleLevel.Set(level)
}

func SetFileLevel(level LogLevel) {
	mu.Lock()
	defer mu.Unlock()
	fileLevel.Set(level)
}

// ParseLevel converts a level string to a LogLevel. Case-insensitive.
// Returns INFO if the string is empty or unrecognized.
func ParseLevel(s string) LogLevel {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return DEBUG
	case "info":
		return INFO
	case "warn", "warning":
		return WARN
	case "error":
		return ERROR
	case "fatal":
		return FATAL
	default:
		return INFO
	}
}

func GetLevel() LogLevel {
	return currentLevel.Level()
}

// SetLevelFromString sets the log level from a string value.
// If the string is empty, the current level is kept.
func SetLevelFromString(s string) {
	if s == "" {
		return
	}
	SetLevel(ParseLevel(s))
}

func EnableFileLogging(filePath string) error {
	mu.Lock()
	defer mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	newFile, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	if logFile != nil {
		logFile.Close()
	}

	logFile = newFile
	fileSlog = slog.New(slog.NewJSONHandler(logFile, &slog.HandlerOptions{
		Level:     &fileLevel,
		AddSource: true,
	}))
	return nil
}

func DisableFileLogging() {
	mu.Lock()
	defer mu.Unlock()

	if logFile != nil {
		logFile.Close()
		logFile = nil
	}
	fileSlog = nil
}

func logMessage(level LogLevel, component string, message string, fields map[string]any) {
	if level < currentLevel.Level() {
		return
	}

	// Build attrs slice: component first, then fields.
	attrs := make([]slog.Attr, 0, len(fields)+1)
	if component != "" {
		attrs = append(attrs, slog.String("component", component))
	}
	for k, v := range fields {
		attrs = append(attrs, slog.Any(k, v))
	}

	// Resolve caller outside the logger so both outputs share it.
	var pcs [1]uintptr
	// skip: runtime.Callers, logMessage, public wrapper (e.g. Info/DebugCF)
	runtime.Callers(3, pcs[:])

	rec := slog.NewRecord(time.Now(), level, message, pcs[0])
	rec.AddAttrs(attrs...)

	mu.RLock()
	cs := consoleSlog
	fs := fileSlog
	mu.RUnlock()

	ctx := context.Background()
	if cs != nil {
		_ = cs.Handler().Handle(ctx, rec)
	}
	if fs != nil {
		_ = fs.Handler().Handle(ctx, rec)
	}

	if level >= FATAL {
		os.Exit(1)
	}
}

func Debug(message string) {
	logMessage(DEBUG, "", message, nil)
}

func DebugC(component string, message string) {
	logMessage(DEBUG, component, message, nil)
}

func Debugf(message string, ss ...any) {
	logMessage(DEBUG, "", fmt.Sprintf(message, ss...), nil)
}

func DebugF(message string, fields map[string]any) {
	logMessage(DEBUG, "", message, fields)
}

func DebugCF(component string, message string, fields map[string]any) {
	logMessage(DEBUG, component, message, fields)
}

func Info(message string) {
	logMessage(INFO, "", message, nil)
}

func InfoC(component string, message string) {
	logMessage(INFO, component, message, nil)
}

func InfoF(message string, fields map[string]any) {
	logMessage(INFO, "", message, fields)
}

func Infof(message string, ss ...any) {
	logMessage(INFO, "", fmt.Sprintf(message, ss...), nil)
}

func InfoCF(component string, message string, fields map[string]any) {
	logMessage(INFO, component, message, fields)
}

func Warn(message string) {
	logMessage(WARN, "", message, nil)
}

func WarnC(component string, message string) {
	logMessage(WARN, component, message, nil)
}

func WarnF(message string, fields map[string]any) {
	logMessage(WARN, "", message, fields)
}

func WarnCF(component string, message string, fields map[string]any) {
	logMessage(WARN, component, message, fields)
}

func Error(message string) {
	logMessage(ERROR, "", message, nil)
}

func ErrorC(component string, message string) {
	logMessage(ERROR, component, message, nil)
}

func Errorf(message string, ss ...any) {
	logMessage(ERROR, "", fmt.Sprintf(message, ss...), nil)
}

func ErrorF(message string, fields map[string]any) {
	logMessage(ERROR, "", message, fields)
}

func ErrorCF(component string, message string, fields map[string]any) {
	logMessage(ERROR, component, message, fields)
}

func Fatal(message string) {
	logMessage(FATAL, "", message, nil)
}

func FatalC(component string, message string) {
	logMessage(FATAL, component, message, nil)
}

func Fatalf(message string, ss ...any) {
	logMessage(FATAL, "", fmt.Sprintf(message, ss...), nil)
}

func FatalF(message string, fields map[string]any) {
	logMessage(FATAL, "", message, fields)
}

func FatalCF(component string, message string, fields map[string]any) {
	logMessage(FATAL, component, message, fields)
}

// --- Color console handler ---

// colorHandler is a slog.Handler that writes colored, human-readable output.
type colorHandler struct {
	w     io.Writer
	level *slog.LevelVar
}

func newColorHandler(w io.Writer, level *slog.LevelVar) *colorHandler {
	return &colorHandler{w: w, level: level}
}

func (h *colorHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

func (h *colorHandler) Handle(_ context.Context, r slog.Record) error {
	if !h.Enabled(context.Background(), r.Level) {
		return nil
	}

	var buf strings.Builder

	// Time
	buf.WriteString("\033[90m")
	buf.WriteString(r.Time.Format("15:04:05"))
	buf.WriteString("\033[0m ")

	// Level with color
	switch {
	case r.Level >= FATAL:
		buf.WriteString("\033[35;1mFTL\033[0m ")
	case r.Level >= ERROR:
		buf.WriteString("\033[31mERR\033[0m ")
	case r.Level >= WARN:
		buf.WriteString("\033[33mWRN\033[0m ")
	case r.Level >= INFO:
		buf.WriteString("\033[32mINF\033[0m ")
	default:
		buf.WriteString("DBG ")
	}

	// Source (caller)
	if r.PC != 0 {
		fs := runtime.CallersFrames([]uintptr{r.PC})
		f, _ := fs.Next()
		if f.File != "" {
			short := filepath.Base(f.File)
			buf.WriteString("\033[1m")
			buf.WriteString(short)
			buf.WriteByte(':')
			fmt.Fprintf(&buf, "%d", f.Line)
			buf.WriteString("\033[0m")
			buf.WriteString("\033[36m >\033[0m ")
		}
	}

	// Message (bold for WARN+)
	if r.Level >= WARN {
		buf.WriteString("\033[1m")
		buf.WriteString(r.Message)
		buf.WriteString("\033[0m")
	} else {
		buf.WriteString(r.Message)
	}

	// Attrs
	r.Attrs(func(a slog.Attr) bool {
		buf.WriteByte(' ')
		buf.WriteString("\033[36m")
		buf.WriteString(a.Key)
		buf.WriteByte('=')
		buf.WriteString("\033[0m")

		v := a.Value.Resolve()
		switch v.Kind() {
		case slog.KindString:
			s := v.String()
			if strings.Contains(s, " ") || strings.Contains(s, "\n") {
				fmt.Fprintf(&buf, "%q", s)
			} else {
				buf.WriteString(s)
			}
		default:
			buf.WriteString(v.String())
		}
		return true
	})

	buf.WriteByte('\n')
	_, err := io.WriteString(h.w, buf.String())
	return err
}

func (h *colorHandler) WithAttrs(_ []slog.Attr) slog.Handler {
	return h // no pre-attached attrs needed for this use case
}

func (h *colorHandler) WithGroup(_ string) slog.Handler {
	return h // no groups needed
}
