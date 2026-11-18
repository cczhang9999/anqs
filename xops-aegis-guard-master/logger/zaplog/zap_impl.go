package zaplog

import (
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type LoggerConfig struct {
	Level           string        `mapstructure:"level"`         // 日志打印级别
	Path            string        `mapstructure:"path"`          // 日志存放路径
	MaxAgeInt       int64         `mapstructure:"max_age"`       // 最大存放
	MaxAge          time.Duration `mapstructure:"-"`             // 最大存放时间
	RotationTimeInt int64         `mapstructure:"rotation_time"` // 日志分割
	RotationTime    time.Duration `mapstructure:"-"`             // 日志分割时间
	Debug           bool          `mapstructure:"debug"`         // 日志打印
	Writefile       bool          `mapstructure:"writefile"`     // 日志存储
}

const (
	Red       = "\x1b[31m"
	Green     = "\x1b[32m"
	Yellow    = "\x1b[33m"
	Blue      = "\x1b[34m"
	Magenta   = "\x1b[35m"
	BlueGreen = "\x1b[36m"

	Reset     = "\x1b[0m"
	Bold      = "\x1b[1m"
	Dim       = "\x1b[2m"
	Italics   = "\x1b[3m"
	Underline = "\x1b[4m"
)

func InitLoggerConfig(c *LoggerConfig) {
	max_age_int := c.MaxAgeInt
	if max_age_int == 0 {
		max_age_int = 7
	}
	c.MaxAge = time.Hour * 24 * time.Duration(max_age_int)
	rotation_time_int := c.RotationTimeInt
	if rotation_time_int == 0 {
		rotation_time_int = 24
	}
	c.RotationTime = time.Hour * time.Duration(rotation_time_int)
}

// LogLevel 日志等级
type LogLevel int

const (
	logDebug LogLevel = iota
	logInfo
	logWarn
	logErr
	logOff
)

var (
	// 字符串和级别映射
	levelMapping = map[string]LogLevel{
		"debug": logDebug,
		"info":  logInfo,
		"warn":  logWarn,
		"err":   logErr,
		"off":   logOff,
	}
)

// ZapLogger zap具体实现接口
type ZapLogger struct {
	Logger   *zap.Logger
	LogLevel LogLevel
	Slog     *zap.SugaredLogger
	msgKey   string
}

// InitLogger 初始化日志框架
func InitLogger(projectName string, conf *LoggerConfig) *ZapLogger {
	if conf == nil {
		conf = &LoggerConfig{}
	}
	InitLoggerConfig(conf)
	myLogger := &ZapLogger{}
	myLogger.Logger = InitZap(projectName, conf.Path, conf.MaxAge, conf.RotationTime, conf.Debug, conf.Writefile)
	myLogger.LogLevel = logInfo
	if level, ok := levelMapping[strings.ToLower(conf.Level)]; ok {
		myLogger.LogLevel = level
	}
	myLogger.Slog = myLogger.Logger.Sugar()
	return myLogger
}

func (z *ZapLogger) handleMsg(v ...interface{}) (msg string, data []zapcore.Field) {
	data = make([]zap.Field, 0, 1)
	for _, arg := range v {
		// 将每个参数转换为 zapcore.Field 类型，并添加到 fields 切片中
		field := zap.Any("", arg)
		data = append(data, field)
	}
	return msg, data
}
func (z *ZapLogger) Debug(v ...interface{}) {
	if z.LogLevel <= logDebug {
		z.Slog.Debug(v...)
	}
}

// Debugf log
func (z *ZapLogger) Debugf(format string, v ...interface{}) {
	if z.LogLevel <= logDebug {
		z.Slog.Debugf(format, v...)
	}
}

// Info log
func (z *ZapLogger) Info(v ...interface{}) {
	if z.LogLevel <= logInfo {
		z.Slog.Info(v...)
	}
}

// Infof log
func (z *ZapLogger) Infof(format string, v ...interface{}) {
	if z.LogLevel <= logInfo {
		z.Slog.Infof(format, v...)
	}
}

// Warn log
func (z *ZapLogger) Warn(v ...interface{}) {
	if z.LogLevel <= logWarn {
		z.Slog.Warn(v...)
	}
}

// Warnf log
func (z *ZapLogger) Warnf(format string, v ...interface{}) {
	if z.LogLevel <= logWarn {
		z.Slog.Warnf(format, v...)
	}
}

// Error log
func (z *ZapLogger) Error(v ...interface{}) {
	if z.LogLevel <= logErr {
		z.Slog.Error(v...)
	}
}

// Errorf log
func (z *ZapLogger) Errorf(format string, v ...interface{}) {
	if z.LogLevel <= logErr {
		z.Slog.Errorf(format, v...)
	}
}
func (z *ZapLogger) Fatal(v ...interface{}) {
	if z.LogLevel <= logErr {
		z.Slog.Fatal(v...)
	}
}
func (z *ZapLogger) Printf(format string, keyvals ...interface{}) {
	z.Slog.Infof(format, keyvals...)
}
