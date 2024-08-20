package log

import (
	"context"

	"github.com/sirupsen/logrus"
)

type ctxKey string

const loggerKey ctxKey = "logger"

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
func Info(ctx context.Context, args ...interface{}) {
	getLogger(ctx).Info(args...)
}

func Infof(ctx context.Context, format string, args ...interface{}) {
	getLogger(ctx).Infof(format, args...)
}

func Debug(ctx context.Context, args ...interface{}) {
	getLogger(ctx).Debug(args...)
}

func Debugf(ctx context.Context, format string, args ...interface{}) {
	getLogger(ctx).Debugf(format, args...)
}

func Warn(ctx context.Context, args ...interface{}) {
	getLogger(ctx).Warn(args...)
}

func Warnf(ctx context.Context, format string, args ...interface{}) {
	getLogger(ctx).Warnf(format, args...)
}

func Error(ctx context.Context, args ...interface{}) {
	getLogger(ctx).Error(args...)
}

func Errorf(ctx context.Context, format string, args ...interface{}) {
	getLogger(ctx).Errorf(format, args...)
}

func Fatal(ctx context.Context, args ...interface{}) {
	getLogger(ctx).Fatal(args...)

}

func Fatalf(ctx context.Context, format string, args ...interface{}) {
	getLogger(ctx).Fatalf(format, args...)

}

func Panic(ctx context.Context, args ...interface{}) {
	getLogger(ctx).Panic(args...)
}

func Panicf(ctx context.Context, format string, args ...interface{}) {
	getLogger(ctx).Panicf(format, args...)
}

func Print(ctx context.Context, args ...interface{}) {
	getLogger(ctx).Print(args...)
}

func SetLevel(ctx context.Context, level logrus.Level) {
	logrus.SetLevel(level)
	getLogger(ctx).Logger.SetLevel(level)
}

func SetFormatter(ctx context.Context, formatter logrus.Formatter) {
	logrus.SetFormatter(formatter)
	getLogger(ctx).Logger.SetFormatter(formatter)
}

func SetCaller(ctx context.Context, flag bool) {
	logrus.SetReportCaller(flag)
	getLogger(ctx).Logger.SetReportCaller(flag)
}
