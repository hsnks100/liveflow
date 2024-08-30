package log

import (
	"context"
	"fmt"
	"runtime"

	"github.com/sirupsen/logrus"
)

type ctxKey string

const loggerKey ctxKey = "logger"

type SkipHook struct {
	skip int
}

func (hook *SkipHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (hook *SkipHook) Fire(entry *logrus.Entry) error {
	pc, file, line, ok := runtime.Caller(hook.skip)
	if ok {
		entry.Caller = &runtime.Frame{
			PC:       pc,
			File:     file,
			Line:     line,
			Function: runtime.FuncForPC(pc).Name(),
		}
	}
	return nil
}

func SetLevel(ctx context.Context, level logrus.Level) {
	logrus.SetLevel(level)
	//getLogger(ctx).Logger.SetLevel(level)
}

func Init() {
	//logrus.AddHook(&SkipHook{skip: 7})
}

func SetFormatter(ctx context.Context, formatter logrus.Formatter) {
	logrus.SetFormatter(formatter)
	//getLogger(ctx).Logger.SetFormatter(formatter)
}

func SetCaller(ctx context.Context, flag bool) {
	//logrus.SetReportCaller(flag)
	//getLogger(ctx).Logger.SetReportCaller(flag)
}

// 컨텍스트에서 로거를 가져오는 헬퍼 함수
func getLogger(ctx context.Context) *logrus.Entry {
	logger, ok := ctx.Value(loggerKey).(*logrus.Entry)
	if !ok {
		// 기본 로거를 반환하거나 오류 처리
		return logrus.NewEntry(logrus.StandardLogger())
	}
	return logger
}

// 로거에 필드를 추가하는 헬퍼 함수
func WithFields(ctx context.Context, fields map[string]interface{}) context.Context {
	logger := getLogger(ctx).WithFields(fields)
	return context.WithValue(ctx, loggerKey, logger)
}

func CallerFileLine() string {
	_, file, line, ok := runtime.Caller(3)
	if ok {
		return fmt.Sprintf("%s:%d", file, line)
	}
	return ""
}

func CallerFunc() string {
	pc, _, _, ok := runtime.Caller(3)
	if ok {
		return runtime.FuncForPC(pc).Name()
	}
	return ""
}

func getLoggerWithStack(ctx context.Context) *logrus.Entry {
	return getLogger(ctx).WithFields(logrus.Fields{
		"file": CallerFileLine(),
		"func": CallerFunc(),
	})
}
func Info(ctx context.Context, args ...interface{}) {
	getLoggerWithStack(ctx).Info(args...)
}

func Infof(ctx context.Context, format string, args ...interface{}) {
	getLoggerWithStack(ctx).Infof(format, args...)
}

func Debug(ctx context.Context, args ...interface{}) {
	getLoggerWithStack(ctx).Debug(args...)
}

func Debugf(ctx context.Context, format string, args ...interface{}) {
	getLoggerWithStack(ctx).Debugf(format, args...)
}

func Warn(ctx context.Context, args ...interface{}) {
	getLoggerWithStack(ctx).Warn(args...)
}

func Warnf(ctx context.Context, format string, args ...interface{}) {
	getLoggerWithStack(ctx).Warnf(format, args...)
}

func Error(ctx context.Context, args ...interface{}) {
	getLoggerWithStack(ctx).Error(args...)
}

func Errorf(ctx context.Context, format string, args ...interface{}) {
	getLoggerWithStack(ctx).Errorf(format, args...)
}

func Fatal(ctx context.Context, args ...interface{}) {
	getLoggerWithStack(ctx).Fatal(args...)

}

func Fatalf(ctx context.Context, format string, args ...interface{}) {
	getLoggerWithStack(ctx).Fatalf(format, args...)
}

func Panic(ctx context.Context, args ...interface{}) {
	getLoggerWithStack(ctx).Panic(args...)
}

func Panicf(ctx context.Context, format string, args ...interface{}) {
	getLoggerWithStack(ctx).Panicf(format, args...)
}

func Print(ctx context.Context, args ...interface{}) {
	getLoggerWithStack(ctx).Print(args...)
}
