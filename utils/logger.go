package utils 

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	Logger *zap.Logger
	loggerOnce sync.Once
)


func InitLogger() {
	loggerOnce.Do(func() {
		if Logger == nil {
			writeSyncer := getLoggerWriter()
			encoder := getEncoder()
			core := zapcore.NewCore(encoder, writeSyncer, zapcore.DebugLevel)
			Logger = zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))
		}
	})
}

// 设置如何写入日志
func getEncoder() zapcore.Encoder {
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	return zapcore.NewConsoleEncoder(encoderConfig)
}


// 设置日志写到哪里
func getLoggerWriter() zapcore.WriteSyncer {
	now := time.Now()
	filename := fmt.Sprintf("./log/%d_%d_%d.log", now.Year(), now.Month(), now.Day())
	file, err := os.OpenFile(filename, os.O_CREATE | os.O_RDWR | os.O_APPEND, 0777)

	if err != nil {
		fmt.Printf("use log file failed: %s", err.Error())
		return zapcore.AddSync(os.Stdout)
	}

	// 利用io.MultiWriter支持多输出目标
	ws := io.MultiWriter(file, os.Stdout)

	return zapcore.AddSync(ws)
}

