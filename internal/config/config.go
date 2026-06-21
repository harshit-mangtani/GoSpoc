package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	Port          int
	LogLevel      string // debug,info,warn,error
	Env           string // dev,prod
	DBHost        string
	DBPort        int
	DBUser        string
	DBPassword    string
	DBName        string
	JWTSecret     string
	RedisAddr     string
	RedisPassword string
	RedisDB       int
	RedisStream   string
	RedisGroup    string
	SweepInterval int // seconds between sweeps for stale queued submissions
	SweepStale    int // seconds a submission may sit as "queued" before re-enqueue
}

func Load() Config {

	if err := godotenv.Load(); err != nil {
		log.Println("no .env found")
	}

	return Config{
		Port:          getInt("PORT", 8080),
		LogLevel:      getStr("LOG_LEVEL", "info"),
		Env:           getStr("ENV", "dev"),
		DBHost:        getStr("POSTGRES_DB_HOST", ""),
		DBPort:        getInt("POSTGRES_DB_PORT", 5432),
		DBUser:        getStr("POSTGRES_DB_USER", ""),
		DBPassword:    getStr("POSTGRES_DB_PASSWORD", ""),
		DBName:        getStr("POSTGRES_DB_NAME", ""),
		JWTSecret:     getStr("JWT_SECRET", ""),
		RedisAddr:     getStr("REDIS_ADDR", ""),
		RedisPassword: getStr("REDIS_PASSWORD", ""),
		RedisDB:       getInt("REDIS_DB", 0),
		RedisStream:   getStr("REDIS_STREAM", "submissions"),
		RedisGroup:    getStr("REDIS_GROUP", "judges"),
		SweepInterval: getInt("SWEEP_INTERVAL_SEC", 30),
		SweepStale:    getInt("SWEEP_STALE_SEC", 60),
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
