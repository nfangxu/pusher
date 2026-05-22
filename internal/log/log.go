package log

import (
	"os"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var sugar *zap.SugaredLogger

func Init(level, path string, maxDays int) error {
	lumberJackLogger := &lumberjack.Logger{
		Filename:   path,
		MaxSize:    100,
		MaxBackups: maxDays,
		MaxAge:     maxDays,
		Compress:   false,
	}

	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "ts"
	encoderConfig.EncodeTime = func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString(t.Format("2006-01-02 15:04:05"))
	}
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder

	core := zapcore.NewTee(
		zapcore.NewCore(
			zapcore.NewJSONEncoder(encoderConfig),
			zapcore.AddSync(lumberJackLogger),
			parseLevel(level),
		),
		zapcore.NewCore(
			zapcore.NewJSONEncoder(encoderConfig),
			zapcore.AddSync(os.Stdout),
			parseLevel(level),
		),
	)

	logger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(0))
	sugar = logger.Sugar()
	return nil
}

func parseLevel(level string) zapcore.Level {
	switch level {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}

func Sugar() *zap.SugaredLogger {
	return sugar
}

func Debug(args ...interface{}) {
	sugar.Debug(args...)
}

func Info(args ...interface{}) {
	sugar.Info(args...)
}

func Warn(args ...interface{}) {
	sugar.Warn(args...)
}

func Error(args ...interface{}) {
	sugar.Error(args...)
}

func Debugf(template string, args ...interface{}) {
	sugar.Debugf(template, args...)
}

func Infof(template string, args ...interface{}) {
	sugar.Infof(template, args...)
}

func Warnf(template string, args ...interface{}) {
	sugar.Warnf(template, args...)
}

func Errorf(template string, args ...interface{}) {
	sugar.Errorf(template, args...)
}

func Infow(msg string, keysAndValues ...interface{}) {
	sugar.Infow(msg, keysAndValues...)
}

func Errorw(msg string, keysAndValues ...interface{}) {
	sugar.Errorw(msg, keysAndValues...)
}

func Fatal(args ...interface{}) {
	sugar.Fatal(args...)
}

func Fatalf(template string, args ...interface{}) {
	sugar.Fatalf(template, args...)
}
