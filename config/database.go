package config

import (
	"gopkg.in/redis.v5"
)

var Database *redis.Options = &redis.Options{
	Addr: "localhost:6379",
}
