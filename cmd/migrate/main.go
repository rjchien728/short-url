package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/joho/godotenv"
)

func main() {
	// 載入 .env 檔案
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using system environment variables")
	}

	// 從環境變數獲取 DSN
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		// 如果沒有直接提供 DSN，則從個別變數組合
		user := os.Getenv("DB_USER")
		pass := os.Getenv("DB_PASSWORD")
		host := "localhost"
		port := os.Getenv("DB_PORT")
		dbname := os.Getenv("DB_NAME")
		if port == "" {
			port = "5432"
		}
		dsn = fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, pass, host, port, dbname)
	}

	m, err := migrate.New("file://migrations", dsn)
	if err != nil {
		log.Fatalf("Failed to initialize migration: %v", err)
	}

	// 處理指令
	flag.Parse()
	command := flag.Arg(0)

	switch command {
	case "up":
		if err := m.Up(); err != nil && err != migrate.ErrNoChange {
			log.Fatalf("Migration UP failed: %v", err)
		}
		fmt.Println("Migration UP successful")
	case "down":
		if err := m.Down(); err != nil && err != migrate.ErrNoChange {
			log.Fatalf("Migration DOWN failed: %v", err)
		}
		fmt.Println("Migration DOWN successful")
	default:
		fmt.Println("Usage: go run cmd/migrate/main.go [up|down]")
	}
}
