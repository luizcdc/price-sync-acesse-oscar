package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/luizcdc/sync-acesse-oscar/oscar/db"
)

var API_KEY string
var VN_PARTNER_ID int64

// TIMEZONE_OFFSET should be in the format -03
var TIMEZONE_OFFSET string
var DEFAULT_PRAZO int
var OSCAR_HOST string
var queryEngine *db.Queries
var connpool *pgxpool.Pool

const APPLICATION_JSON = "application/json"

var SERVER_PORT uint16

// loadEnv loads the environment variables from the .env file
func loadEnv() {
	if godotenv.Load() != nil {
		log.Fatalln("Error loading .env file")
	}

	API_KEY = os.Getenv("API_KEY")
	if API_KEY == "" {
		log.Fatalln("API_KEY not found in environment")
	}

	TIMEZONE_OFFSET = os.Getenv("TIMEZONE_OFFSET")
	if TIMEZONE_OFFSET == "" {
		log.Fatalln("TIMEZONE_OFFSET not found in environment")
	}
	p_id, err := strconv.Atoi(os.Getenv("VN_PARTNER_ID"))
	if err != nil {
		log.Fatalf("Error parsing VN_PARTNER_ID: %v", err)
	}
	VN_PARTNER_ID = int64(p_id)
}

func main() {
	loadEnv()

	setUpDB()

	StartRouter()

}

// setUpDB initializes the database connection pool and the query engine singletons
func setUpDB() {
	ctx := context.TODO()
	connConfig, err := pgxpool.ParseConfig(os.Getenv("POSTGRES_URL"))
	if err != nil {
		log.Fatalln(err)
	}
	connConfig.MaxConnIdleTime = 30 * time.Second
	connConfig.MaxConns = 10
	connpool, err = pgxpool.NewWithConfig(
		ctx,
		connConfig,
	)
	if err != nil {
		log.Fatalln(err)
	}
	defer connpool.Close()

	queryEngine = db.New(connpool)
}
