package utils

import (
	"os"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Mysql  *Mysql  `toml:"mysql"`
	Server *Server `toml:"server"`
	Redis  *Redis  `toml:"redis"`
	Kafka  *Kafka  `toml:"kafka"`
}

type Kafka struct {
	Addr  string `toml:"addr"`
	Topic string `toml:"topic"`
}

type Mysql struct {
	DBHost     string `toml:"db_host"`
	DBPort     string `toml:"db_port"`
	DBUser     string `toml:"db_user"`
	DBPassword string `toml:"db_password"`
	DBName     string `toml:"db_name"`
}

type Redis struct {
	Addr		string `toml:"addr"`
	DB			int `toml:"db"`
}

type Server struct {
	AppMode  string `toml:"app_mode"`
	HttpPort string	`toml:"http_port"`
	JwtKey   string `toml:"jwt_key"`
}

var Conf Config

// 初始化配置，优先读取环境变量 LOTTERY_CONFIG，默认 config/config.toml
func init() {
	path := os.Getenv("LOTTERY_CONFIG")
	if path == "" {
		path = "config/config.toml"
	}
	_, err := toml.DecodeFile(path, &Conf)
	if err != nil {
		panic(err)
	}
}
