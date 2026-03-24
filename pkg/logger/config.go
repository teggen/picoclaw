package logger

// LoggingConfig mirrors the config.LoggingConfig structure to avoid import cycles.
type LoggingConfig struct {
	Level       string
	FileLogging FileLogConfig
	Console     ConsoleLogConfig
}

type FileLogConfig struct {
	Enabled bool
	Path    string
	Level   string
}

type ConsoleLogConfig struct {
	Level string
}

// ApplyConfig applies logging configuration settings.
// The debug parameter indicates whether the CLI --debug flag was set,
// which takes precedence over config values.
func ApplyConfig(cfg LoggingConfig, debug bool) {
	if debug {
		cfg.Level = "debug"
	}

	globalLevel, _ := ParseLevel(cfg.Level)

	// Determine effective levels for each output
	consoleLevel := globalLevel
	if cfg.Console.Level != "" {
		consoleLevel, _ = ParseLevel(cfg.Console.Level)
	}

	fileLevel := globalLevel
	if cfg.FileLogging.Level != "" {
		fileLevel, _ = ParseLevel(cfg.FileLogging.Level)
	}

	// Set global level to the minimum of all active outputs so messages
	// reach logMessage() and each output can filter independently.
	minLevel := consoleLevel
	if cfg.FileLogging.Enabled && fileLevel < minLevel {
		minLevel = fileLevel
	}
	SetLevel(minLevel)

	// Set independent output levels
	SetConsoleLevel(consoleLevel)

	// Enable or disable file logging based on configuration
	if cfg.FileLogging.Enabled {
		if err := EnableFileLogging(cfg.FileLogging.Path); err != nil {
			Errorf("Failed to enable file logging at %s: %v", cfg.FileLogging.Path, err)
			return
		}
		SetFileLevel(fileLevel)
	} else {
		DisableFileLogging()
	}
}
