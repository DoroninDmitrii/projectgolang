package config

type loggerConfig struct {
	LogLevel string `env:"LOGGER_LEVEL" env-default:"info"`
}

func (c *loggerConfig) Level() string {
	return c.LogLevel
}
