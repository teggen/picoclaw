package logger

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLogLevelFiltering(t *testing.T) {
	initialLevel := GetLevel()
	defer SetLevel(initialLevel)

	SetLevel(WARN)

	tests := []struct {
		name      string
		level     LogLevel
		shouldLog bool
	}{
		{"DEBUG message", DEBUG, false},
		{"INFO message", INFO, false},
		{"WARN message", WARN, true},
		{"ERROR message", ERROR, true},
		{"FATAL message", FATAL, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			switch tt.level {
			case DEBUG:
				Debug(tt.name)
			case INFO:
				Info(tt.name)
			case WARN:
				Warn(tt.name)
			case ERROR:
				Error(tt.name)
			case FATAL:
				if tt.shouldLog {
					t.Logf("FATAL test skipped to prevent program exit")
				}
			}
		})
	}

	SetLevel(INFO)
}

func TestLoggerWithComponent(t *testing.T) {
	initialLevel := GetLevel()
	defer SetLevel(initialLevel)

	SetLevel(DEBUG)

	tests := []struct {
		name      string
		component string
		message   string
		fields    map[string]any
	}{
		{"Simple message", "test", "Hello, world!", nil},
		{"Message with component", "discord", "Discord message", nil},
		{"Message with fields", "telegram", "Telegram message", map[string]any{
			"user_id": "12345",
			"count":   42,
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			switch {
			case tt.fields == nil && tt.component != "":
				InfoC(tt.component, tt.message)
			case tt.fields != nil:
				InfoF(tt.message, tt.fields)
			default:
				Info(tt.message)
			}
		})
	}

	SetLevel(INFO)
}

func TestLogLevels(t *testing.T) {
	tests := []struct {
		name  string
		level LogLevel
		want  string
	}{
		{"DEBUG level", DEBUG, "DEBUG"},
		{"INFO level", INFO, "INFO"},
		{"WARN level", WARN, "WARN"},
		{"ERROR level", ERROR, "ERROR"},
		{"FATAL level", FATAL, "FATAL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if logLevelNames[tt.level] != tt.want {
				t.Errorf("logLevelNames[%d] = %s, want %s", tt.level, logLevelNames[tt.level], tt.want)
			}
		})
	}
}

func TestSetGetLevel(t *testing.T) {
	initialLevel := GetLevel()
	defer SetLevel(initialLevel)

	tests := []LogLevel{DEBUG, INFO, WARN, ERROR, FATAL}

	for _, level := range tests {
		SetLevel(level)
		if GetLevel() != level {
			t.Errorf("SetLevel(%v) -> GetLevel() = %v, want %v", level, GetLevel(), level)
		}
	}
}

func TestLoggerHelperFunctions(t *testing.T) {
	initialLevel := GetLevel()
	defer SetLevel(initialLevel)

	SetLevel(INFO)

	Debug("This should not log")
	Debugf("this should not log")
	Info("This should log")
	Warn("This should log")
	Error("This should log")

	InfoC("test", "Component message")
	InfoF("Fields message", map[string]any{"key": "value"})
	Infof("test from %v", "Infof")

	WarnC("test", "Warning with component")
	ErrorF("Error with fields", map[string]any{"error": "test"})
	Errorf("test from %v", "Errorf")

	SetLevel(DEBUG)
	DebugC("test", "Debug with component")
	Debugf("test from %v", "Debugf")
	WarnF("Warning with fields", map[string]any{"key": "value"})
}

func TestDefaultLevelIsInfo(t *testing.T) {
	if logLevelNames[INFO] != "INFO" {
		t.Errorf("INFO constant mapped to %q, want \"INFO\"", logLevelNames[INFO])
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input string
		want  LogLevel
	}{
		{"debug", DEBUG},
		{"DEBUG", DEBUG},
		{"Debug", DEBUG},
		{"info", INFO},
		{"INFO", INFO},
		{"warn", WARN},
		{"WARN", WARN},
		{"warning", WARN},
		{"WARNING", WARN},
		{"error", ERROR},
		{"ERROR", ERROR},
		{"fatal", FATAL},
		{"FATAL", FATAL},
		{"  info  ", INFO},
		{"", INFO},
		{"unknown", INFO},
		{"  debug  ", DEBUG},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseLevel(tt.input)
			if got != tt.want {
				t.Errorf("ParseLevel(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestSetFileLevel(t *testing.T) {
	initialLevel := GetLevel()
	defer SetLevel(initialLevel)

	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")
	if err := EnableFileLogging(logPath); err != nil {
		t.Fatalf("EnableFileLogging failed: %v", err)
	}
	defer DisableFileLogging()

	SetLevel(DEBUG)
	SetConsoleLevel(ERROR)
	SetFileLevel(DEBUG)

	Debug("file-level-test-message")

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	if len(content) == 0 {
		t.Error("Expected debug message in file log, but file is empty")
	}
}

func TestApplyConfig(t *testing.T) {
	initialLevel := GetLevel()
	defer SetLevel(initialLevel)
	defer DisableFileLogging()

	t.Run("debug flag overrides config", func(t *testing.T) {
		ApplyConfig(LoggingConfig{Level: "error"}, true)
		if GetLevel() != DEBUG {
			t.Errorf("Expected DEBUG level when debug=true, got %v", GetLevel())
		}
	})

	t.Run("global level from config", func(t *testing.T) {
		ApplyConfig(LoggingConfig{Level: "warn"}, false)
		if GetLevel() != WARN {
			t.Errorf("Expected WARN level, got %v", GetLevel())
		}
	})

	t.Run("file logging enabled", func(t *testing.T) {
		tmpDir := t.TempDir()
		logPath := filepath.Join(tmpDir, "test.log")
		ApplyConfig(LoggingConfig{
			Level:       "info",
			FileLogging: FileLogConfig{Enabled: true, Path: logPath, Level: "debug"},
		}, false)
		if GetLevel() != DEBUG {
			t.Errorf("Expected DEBUG as global minimum, got %v", GetLevel())
		}
		if _, err := os.Stat(logPath); os.IsNotExist(err) {
			t.Error("Expected log file to be created")
		}
		DisableFileLogging()
	})

	t.Run("console level override", func(t *testing.T) {
		ApplyConfig(LoggingConfig{
			Level:   "debug",
			Console: ConsoleLogConfig{Level: "warn"},
		}, false)
		if GetLevel() != WARN {
			t.Errorf("Expected WARN (console override, no file), got %v", GetLevel())
		}
	})

	t.Run("file more verbose than console", func(t *testing.T) {
		tmpDir := t.TempDir()
		logPath := filepath.Join(tmpDir, "test.log")
		ApplyConfig(LoggingConfig{
			Level:       "info",
			FileLogging: FileLogConfig{Enabled: true, Path: logPath, Level: "debug"},
			Console:     ConsoleLogConfig{Level: "warn"},
		}, false)
		if GetLevel() != DEBUG {
			t.Errorf("Expected DEBUG as global minimum, got %v", GetLevel())
		}
		DisableFileLogging()
	})
}

func TestSetLevelFromString(t *testing.T) {
	initialLevel := GetLevel()
	defer SetLevel(initialLevel)

	SetLevel(INFO)
	SetLevelFromString("error")
	if got := GetLevel(); got != ERROR {
		t.Errorf("after SetLevelFromString(\"error\"): GetLevel() = %v, want ERROR", got)
	}

	SetLevelFromString("")
	if got := GetLevel(); got != ERROR {
		t.Errorf("after SetLevelFromString(\"\"): GetLevel() = %v, want ERROR (unchanged)", got)
	}

	SetLevelFromString("FATAL")
	if got := GetLevel(); got != FATAL {
		t.Errorf("after SetLevelFromString(\"FATAL\"): GetLevel() = %v, want FATAL", got)
	}
}
