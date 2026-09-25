package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port        string
	PostgresURL string
	RedisAddr   string
	RedisPass   string
	JWTSecret   string
	JWTHours    int
	MaxFreeOCR  int64
	S3Endpoint  string
	S3AccessKey string
	S3SecretKey string
	S3Bucket    string
	S3UseSSL    bool
	GeminiAPIKey string
	GeminiModel  string
}

func LoadConfig() *Config {
	return &Config{
		Port:         getEnv("PORT", "8080"),
		PostgresURL:  getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/scannerapp?sslmode=disable"),
		RedisAddr:    getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPass:    getEnv("REDIS_PASSWORD", ""),
		JWTSecret:    getEnv("JWT_SECRET", "super-secret-jwt-key-change-in-production"),
		JWTHours:     getEnvAsInt("JWT_EXPIRATION_HOURS", 72),
		MaxFreeOCR:   int64(getEnvAsInt("MAX_FREE_OCR", 5)),
		S3Endpoint:   getEnv("S3_ENDPOINT", "localhost:9000"),
		S3AccessKey:  getEnv("S3_ACCESS_KEY", "minioadmin"),
		S3SecretKey:  getEnv("S3_SECRET_KEY", "minioadmin"),
		S3Bucket:     getEnv("S3_BUCKET", "scans-bucket"),
		S3UseSSL:     getEnvAsBool("S3_USE_SSL", false),
		GeminiAPIKey: getEnv("GEMINI_API_KEY", ""),
		GeminiModel:  getEnv("GEMINI_MODEL", "gemini-1.5-flash-latest"),
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvAsInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvAsBool(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			return b
		}
	}
	return defaultVal
}
