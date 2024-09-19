package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/luizcdc/sync-acesse-oscar/acesse/db"
)

var API_KEY string

// TIMEZONE_OFFSET should be in the format -03:00
var TIMEZONE_OFFSET string
var DEFAULT_PRAZO int
var HOURS_BETWEEN_NOTIFICATIONS int
var EMAIL_AUTH smtp.Auth
var EMAIL_SMTP_SERVER_ADDR string
var EMAIL_FROM_FORMATTED string
var EMAIL_FROM string
var EMAIL_ADMIN_ADDRESS string
var OSCAR_HOST string
var API_PREFIX string

const APPLICATION_JSON = "application/json"

var SERVER_PORT uint16

type PriceToUpdate struct {
	Codigo float64 `json:"codigo"`
	Preco  float64 `json:"preco"`
}

// loadEnv loads the environment variables from the .env file.
func loadEnv() {
	if godotenv.Load() != nil {
		log.Fatal("Error loading .env file")
	}

	API_KEY = os.Getenv("API_KEY")
	if API_KEY == "" {
		log.Fatal("API_KEY not found in environment")
	}

	TIMEZONE_OFFSET = os.Getenv("TIMEZONE_OFFSET")
	if TIMEZONE_OFFSET == "" {
		log.Fatal("TIMEZONE_OFFSET not found in environment")
	}
	var err error
	DEFAULT_PRAZO, err = strconv.Atoi(os.Getenv("DEFAULT_PRAZO"))
	if err != nil {
		log.Fatal("DEFAULT_PRAZO not found in environment")
	}

	HOURS_BETWEEN_NOTIFICATIONS, err = strconv.Atoi(os.Getenv("HOURS_BETWEEN_NOTIFICATIONS"))
	if err != nil {
		log.Fatal("HOURS_BETWEEN_NOTIFICATIONS not found in environment")
	}

	EMAIL_FROM_FORMATTED = fmt.Sprintf("%s <%s>", os.Getenv("EMAIL_FROM_NAME"), os.Getenv("EMAIL_FROM"))
	EMAIL_FROM = os.Getenv("EMAIL_FROM")
	EMAIL_SMTP_SERVER_ADDR = fmt.Sprintf("%s:%s", os.Getenv("EMAIL_HOSTNAME"), os.Getenv("EMAIL_PORT"))

	EMAIL_ADMIN_ADDRESS = os.Getenv("EMAIL_ADMIN_ADDRESS")
	if EMAIL_ADMIN_ADDRESS == "" {
		log.Fatal("EMAIL_ADMIN_ADDRESS not found in environment")
	}
	EMAIL_AUTH = smtp.PlainAuth(
		"",
		os.Getenv("EMAIL_FROM"),
		os.Getenv("EMAIL_PASSWORD"),
		os.Getenv("EMAIL_HOSTNAME"),
	)
	OSCAR_HOST = os.Getenv("OSCAR_HOST")
	if OSCAR_HOST == "" {
		log.Fatal("OSCAR_HOST not found in environment")
	}

	API_PREFIX = os.Getenv("API_PREFIX")
	if API_PREFIX == "" {
		log.Fatal("API_PREFIX not found in environment")
	}
}

func main() {
	loadEnv()
	closing := make(chan interface{})
	updateQueue := make(chan float64)
	ctx := context.TODO()
	connConfig, err := pgxpool.ParseConfig(os.Getenv("POSTGRES_URL"))
	if err != nil {
		log.Fatal(err)
	}
	connConfig.MaxConnIdleTime = 30 * time.Second
	connConfig.MaxConns = 3
	connpool, err := pgxpool.NewWithConfig(
		ctx,
		connConfig,
	)
	if err != nil {
		log.Fatal(err)
	}
	defer connpool.Close()

	queryEngine := db.New(connpool)

	go UpdaterGoroutine(connpool, queryEngine, closing, updateQueue)
	minutesToSleep := 15
	for {
		select {
		case <-closing:
			log.Println("Closing gracefully...")
			return
		default:
			log.Println("Checking for updates.")
		}
		if isThereNewUpdate(queryEngine, closing) {
			log.Println("There is a new update!")
			go notifyOfUpdate(queryEngine, connpool, updateQueue, closing)
		}
		log.Println("Clearing old notifications and price watchers...")
		queryEngine.ClearNotificationEvents(ctx, HOURS_BETWEEN_NOTIFICATIONS)
		go queryEngine.RemoveOldPriceWatchers(ctx)
		log.Printf("Sleeping for %v minutes...", minutesToSleep)
		time.Sleep(time.Duration(minutesToSleep) * time.Minute)
	}
}

// calculateCurrentHash calculates the hash of all products' prices
// and returns it as a string.
func calculateCurrentHash(queryEngine *db.Queries) (string, error) {
	products, err := queryEngine.GetAllProducts(context.TODO(), DEFAULT_PRAZO)
	if err != nil {
		return "", err
	}
	var allProductsString bytes.Buffer
	for _, product := range products {
		allProductsString.WriteString(fmt.Sprintf("%v%v", product.CodigoItem, product.Preco))
	}
	finalHash := fmt.Sprintf("%x", sha256.Sum256(allProductsString.Bytes()))
	return finalHash, nil
}

// isThereNewUpdate compares the last stored hash and the calculated current hash
// of all products' prices, signaling whether there has been any update or not.
func isThereNewUpdate(queryEngine *db.Queries, closingChannel chan interface{}) bool {
	watcher, err := queryEngine.GetPriceWatcher(context.TODO())
	if err != nil {
		defer closeChannel(closingChannel)
		log.Fatal(err)
	}

	currentHash, err := calculateCurrentHash(queryEngine)
	if err != nil {
		defer closeChannel(closingChannel)
		log.Fatal(err)
	}
	log.Printf("Current hash[:16]: '%s', Last hash[:16]: '%s'\n", currentHash[:min(16, len(currentHash))], watcher.PricesHash[:min(16, len(watcher.PricesHash))])
	return watcher.PricesHash != currentHash
}

// readBody reads the body of an http.Response and returns it as a string.
func readBody(resp *http.Response) string {
	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)
	return buf.String()
}

// notifyOfUpdate notifies Oscar that there has been an update of the prices.
// If the update is acknowledged but refused, the last update time and hash are updated.
// If the update is acknowledged and accepted, it sends the product codes provided in the answer
// to the updateQueue channel.
func notifyOfUpdate(queryEngine *db.Queries, connpool *pgxpool.Pool, updateQueue chan float64, closing chan interface{}) {
	client := http.Client{
		Timeout: 15 * time.Second,
	}
	mostRecentUpdate, err := queryEngine.GetMostRecentUpdate(context.TODO())
	if err != nil {
		defer closeChannel(closing)
		log.Fatal(err)
	}
	// We send the most recent update as 23:59:59 of the recorded day because the acesse database
	// only stores the date.
	lastUpdateTimestamp := url.QueryEscape(fmt.Sprintf("%s 23:59:59%s", mostRecentUpdate.Time.Format("2006-01-02"), TIMEZONE_OFFSET))

	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s%sprice-update/catalogue-product?lastupdate=%s", OSCAR_HOST, API_PREFIX, lastUpdateTimestamp), nil)
	if err != nil {
		defer closeChannel(closing)
		log.Fatal(err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", API_KEY))
	resp, err := client.Do(req)
	if err != nil {
		// if the error is due to no connection, return. Else, close the channel and log the error
		if strings.Contains(err.Error(), "dial tcp") {
			go NotifyOfNoConnection(queryEngine, connpool)
			return
		}
		defer closeChannel(closing)
		log.Fatal(err)
	}
	defer resp.Body.Close()
	log.Println("Notification of pending update response status:", resp.Status)
	if resp.StatusCode != http.StatusOK {
		go NotifyErrorResponse(queryEngine, connpool, resp)
		return
	}
	text := strings.Trim(readBody(resp), "\"'")

	if text == "null" {
		hash, err := calculateCurrentHash(queryEngine)
		if err != nil {
			defer closeChannel(closing)
			log.Fatal(err)
		}
		queryEngine.UpdatePriceWatcher(context.TODO(), hash)
		return
	}

	for _, codigo := range strings.Split(text, ",") {
		codigoFloat, err := strconv.ParseFloat(codigo, 64)
		if err != nil {
			defer closeChannel(closing)
			log.Fatal(err)
		}
		updateQueue <- codigoFloat
	}
	updateQueue <- -1.0 // Signals that the current update has finished sending the list of codes
}

// closeChannel closes a channel if it is not already closed.
func closeChannel(toClose chan interface{}) {
	select {
	case <-toClose:
		return
	default:
		close(toClose)
	}
}

// UpdaterGoroutine receives the product codes to update from the updateQueue channel,
// collects their respective prices and sends the updates to Oscar.
func UpdaterGoroutine(connpool *pgxpool.Pool, queryEngine *db.Queries, closing chan interface{}, updateQueue chan float64) {
	codigos := make([]float64, 0, 500)
	for {
		select {
		case <-closing:
			log.Println("Closing UpdaterGoroutine after closing channel was closed...")
			return
		case codigo := <-updateQueue:
			if codigo != -1.0 {
				codigos = append(codigos, codigo)
			} else {
				// SEND ALL UPDATES TO OSCAR
				prepareUpdate(queryEngine, codigos, connpool)
				codigos = make([]float64, 0, 500)
			}
		}
	}
}

// prepareUpdate uses the slice of codes to get the prices from the database and prepare to
// send them to Oscar.
func prepareUpdate(queryEngine *db.Queries, codigos []float64, connpool *pgxpool.Pool) {
	products, err := queryEngine.GetProductsPrices(context.TODO(), db.GetProductsPricesParams{CodigoPrazo: DEFAULT_PRAZO, CodigosItens: codigos})
	if err != nil {
		log.Fatal(err)
	}
	pricesToSend := make([]PriceToUpdate, 0, 500)

	// Check for duplicate codigo_item
	// The query is ordered by codigo_item and alteracao_preco, so we can check for duplicates
	// by only looking at the next item
	var lastCodigo float64
	var lastCodigoUnidade int
	var lastDataAlteracao time.Time
	for _, product := range products {
		if product.CodigoItem == lastCodigo {
			if product.AlteracaoPreco.Time.Format("2006-01-02") == lastDataAlteracao.Format("2006-01-02") {
				// skipping
				if len(pricesToSend) > 0 {
					pricesToSend = pricesToSend[:len(pricesToSend)-1]
				}
				go NotifySimultaneousPriceChanges(queryEngine, product, connpool, lastCodigoUnidade)
			}
		} else {
			pricesToSend = append(pricesToSend, PriceToUpdate{Codigo: product.CodigoItem, Preco: product.Preco})
			lastCodigo = product.CodigoItem
			lastCodigoUnidade = product.CodigoUnidade
			lastDataAlteracao = product.AlteracaoPreco.Time
		}
	}

	err = sendUpdate(queryEngine, connpool, pricesToSend)
	if err != nil {
		log.Println(err)
		return
	}
	hash, err := calculateCurrentHash(queryEngine)
	if err != nil {
		log.Println(err)
		return
	}
	queryEngine.UpdatePriceWatcher(context.TODO(), hash)
}

// sendUpdate sends the prices to Oscar and returns an error if the request fails.
func sendUpdate(queryEngine *db.Queries, connpool *pgxpool.Pool, prices []PriceToUpdate) error {
	client := http.Client{
		Timeout: 15 * time.Second,
	}
	jsonPrices, err := json.Marshal(prices)
	if err != nil {
		log.Println(err)
		return err
	}
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s%sprice-update/catalogue-product", OSCAR_HOST, API_PREFIX), bytes.NewBuffer(jsonPrices))
	if err != nil {
		log.Println(err)
		return err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", API_KEY))
	resp, err := client.Do(req)
	if err != nil {
		if strings.Contains(err.Error(), "dial tcp") {
			go NotifyOfNoConnection(queryEngine, connpool)
			return err
		}
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("failure Oscar reported an error while receiving the update: %v", resp.Status)
		go NotifyErrorResponse(queryEngine, connpool, resp)
		return err
	}
	log.Printf("Update sent successfully. Response status: %s\n", resp.Status)
	return nil
}
