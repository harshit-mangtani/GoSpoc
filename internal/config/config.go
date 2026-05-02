package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type config struct {
	Port     int
	LogLevel string // debug,info,warn,error
	Env      string // dev,prod
}

func Load() config {

	if err := godotenv.Load(); err != nil {
		log.Println("no .env found")
	}

	return config{
		Port:     getInt("PORT", 8080),
		LogLevel: getStr("LOG_LEVEL", "info"),
		Env:      getStr("ENV", "dev"),
	}
}

func getInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			return p
		}
	}
	return def
}

func getStr(key string, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
