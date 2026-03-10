package logger

import (
	"fmt"
	"log"
	"os"
	"time"
)

type LogLevel int

const (
	DebugLevel LogLevel = iota
	InfoLevel
	WarnLevel
	ErrorLevel
)

type Logger interface {
	Debug(msg string, fields ...Field)
	Info(msg string, fields ...Field)
	Warn(msg string, fields ...Field)
	Error(msg string, fields ...Field)
}

type Field struct {
	Key   string
	Value interface{}
}

func String(key, value string) Field {
	return Field{Key: key, Value: value}
}

func Int(key string, value int) Field {
	return Field{Key: key, Value: value}
}

func Int64(key string, value int64) Field {
	return Field{Key: key, Value: value}
}

func Float64(key string, value float64) Field {
	return Field{Key: key, Value: value}
}

func Bool(key string, value bool) Field {
	return Field{Key: key, Value: value}
}

func Err(err error) Field {
	if err == nil {
		return Field{Key: "error", Value: nil}
	}
	return Field{Key: "error", Value: err.Error()}
}

type StdLogger struct {
	level LogLevel
	logger *log.Logger
}

func NewStdLogger(level LogLevel) *StdLogger {
	return &StdLogger{
		level: level,
		logger: log.New(os.Stdout, "", 0),
	}
}

func (l *StdLogger) formatFields(fields []Field) string {
	if len(fields) == 0 {
		return ""
	}
	var sb string
	for i, f := range fields {
		if i > 0 {
			sb += ", "
		}
		sb += fmt.Sprintf("%s=%v", f.Key, f.Value)
	}
	return " | " + sb
}

func (l *StdLogger) Debug(msg string, fields ...Field) {
	if l.level <= DebugLevel {
		l.logger.Printf("[DEBUG] %s %s%s\n", time.Now().Format("2006-01-02 15:04:05"), msg, l.formatFields(fields))
	}
}

func (l *StdLogger) Info(msg string, fields ...Field) {
	if l.level <= InfoLevel {
		l.logger.Printf("[INFO]  %s %s%s\n", time.Now().Format("2006-01-02 15:04:05"), msg, l.formatFields(fields))
	}
}

func (l *StdLogger) Warn(msg string, fields ...Field) {
	if l.level <= WarnLevel {
		l.logger.Printf("[WARN]  %s %s%s\n", time.Now().Format("2006-01-02 15:04:05"), msg, l.formatFields(fields))
	}
}

func (l *StdLogger) Error(msg string, fields ...Field) {
	if l.level <= ErrorLevel {
		l.logger.Printf("[ERROR] %s %s%s\n", time.Now().Format("2006-01-02 15:04:05"), msg, l.formatFields(fields))
	}
}

var defaultLogger Logger = NewStdLogger(InfoLevel)

func SetDefault(l Logger) {
	defaultLogger = l
}

func Default() Logger {
	return defaultLogger
}

func Debug(msg string, fields ...Field) {
	defaultLogger.Debug(msg, fields...)
}

func Info(msg string, fields ...Field) {
	defaultLogger.Info(msg, fields...)
}

func Warn(msg string, fields ...Field) {
	defaultLogger.Warn(msg, fields...)
}

func Error(msg string, fields ...Field) {
	defaultLogger.Error(msg, fields...)
}
