package logger

import "testing"

func TestLogger(t *testing.T) {
	l := NewStdLogger(DebugLevel)
	
	l.Debug("debug test")
	l.Info("info test")
	l.Warn("warn test")
	l.Error("error test")
	
	l.Info("test with fields", String("key", "value"), Int("num", 123), Err(nil))
}

func TestPackageFunctions(t *testing.T) {
	Debug("package debug")
	Info("package info")
	Warn("package warn")
	Error("package error")
	
	Info("package test with fields", String("key", "value"), Int("num", 123), Err(nil))
}
