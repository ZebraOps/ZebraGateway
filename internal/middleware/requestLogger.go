package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// RequestLogger 请求日志中间件，记录每次请求的方法、路径、状态码、耗时等信息
func RequestLogger(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		status := c.Writer.Status()
		fields := []zap.Field{
			zap.Int("status", status),
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.String("query", c.Request.URL.RawQuery),
			zap.String("ip", c.ClientIP()),
			zap.String("user_agent", c.Request.UserAgent()),
			zap.Duration("duration", time.Since(start)),
		}

		if errMsg := c.Errors.ByType(gin.ErrorTypePrivate).String(); errMsg != "" {
			fields = append(fields, zap.String("error", errMsg))
		}

		switch {
		case status >= 500:
			logger.Error("服务端错误", fields...)
		case status >= 400:
			logger.Warn("客户端错误", fields...)
		default:
			logger.Info("请求处理完成", fields...)
		}
	}
}
