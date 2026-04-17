package log

import (
	"os"

	"github.com/ZebraOps/ZebraGateway/internal/types"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var logger *zap.Logger
var sugar *zap.SugaredLogger

// InitWithConfig 根据配置初始化日志系统（支持文件轮转 + 控制台输出）
func InitWithConfig(cfg types.LoggingConfig) error {
	var writers []zapcore.WriteSyncer

	for _, path := range cfg.OutputPaths {
		switch path {
		case "stdout":
			writers = append(writers, zapcore.AddSync(os.Stdout))
		case "stderr":
			writers = append(writers, zapcore.AddSync(os.Stderr))
		default:
			// 文件输出，启用 lumberjack 日志轮转
			writers = append(writers, zapcore.AddSync(&lumberjack.Logger{
				Filename:   path,
				MaxSize:    cfg.MaxSize,
				MaxAge:     cfg.MaxAge,
				MaxBackups: cfg.MaxBackups,
				Compress:   cfg.Compress,
			}))
		}
	}

	var level zapcore.Level
	if err := level.UnmarshalText([]byte(cfg.Level)); err != nil {
		level = zapcore.InfoLevel
	}

	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	var encoder zapcore.Encoder
	if cfg.Encoding == "console" {
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	} else {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	}

	core := zapcore.NewCore(
		encoder,
		zapcore.NewMultiWriteSyncer(writers...),
		level,
	)

	logger = zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
	sugar = logger.Sugar()
	zap.ReplaceGlobals(logger)
	return nil
}

// Sync 刷新缓冲日志
func Sync() {
	if logger != nil {
		_ = logger.Sync()
	}
}

// L 返回结构化 logger
func L() *zap.Logger {
	return logger
}

// S 返回 SugaredLogger（支持 Printf 风格）
func S() *zap.SugaredLogger {
	return sugar
}
