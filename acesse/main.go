package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
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
var EMAIL_ADMIN_ADDRESS string
var OSCAR_HOST string

const APPLICATION_JSON = "application/json"

var SERVER_PORT uint16

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
	EMAIL_SMTP_SERVER_ADDR = fmt.Sprintf("%s:%s", os.Getenv("EMAIL_SMTP_SERVER"), os.Getenv("EMAIL_SMTP_PORT"))

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

	return watcher.PricesHash != currentHash
}

func readBody(resp *http.Response) string {
	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)
	return buf.String()
}

// notifyOfUpdate notifies Oscar that there has been an update of the prices.
// If the update is acknowledged but refused, the last update time and hash are updated.
// If the update is acknowledged and accepted, it sends the product codes provided in the answer
// to the updateQueue channel.
func notifyOfUpdate(queryEngine *db.Queries, updateQueue chan float64, closing chan interface{}) {
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
	lastUpdateTimestamp := url.QueryEscape(fmt.Sprintf("%sT23:59:59%s", mostRecentUpdate.Time.Format("2006-01-02"), TIMEZONE_OFFSET))
	resp, err := client.Get(fmt.Sprintf("%s/api/integration/price-update/catalogue-product?lastupdate=%s&key=%s", OSCAR_HOST, lastUpdateTimestamp, url.QueryEscape(API_KEY)))
	if err != nil {
		defer closeChannel(closing)
		log.Fatal(err)
	}
	defer resp.Body.Close()

	text := strings.Trim(readBody(resp), "\"")

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

func closeChannel(toClose chan interface{}) {
	select {
	case <-toClose:
		return
	default:
		close(toClose)
	}
}

func UpdaterGoroutine(connpool *pgxpool.Pool, queryEngine *db.Queries, closing chan interface{}, updateQueue chan float64) {
	codigos := make([]float64, 0, 500)
	for {
		select {
		case <-closing:
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

func prepareUpdate(queryEngine *db.Queries, codigos []float64, connpool *pgxpool.Pool) {
	type PriceUpdate struct {
		Codigo float64 `json:"codigo"`
		Preco  float64 `json:"preco"`
	}
	products, err := queryEngine.GetProductsPrices(context.TODO(), db.GetProductsPricesParams{CodigoPrazo: DEFAULT_PRAZO, CodigosItens: codigos})
	if err != nil {
		log.Fatal(err)
	}
	pricesToSend := make([]PriceUpdate, 0, 500)

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
				notifySimultaneousPriceChanges(queryEngine, product, connpool, lastCodigoUnidade)
			}
		} else {
			pricesToSend = append(pricesToSend, PriceUpdate{Codigo: product.CodigoItem, Preco: product.Preco})
			lastCodigo = product.CodigoItem
			lastCodigoUnidade = product.CodigoUnidade
			lastDataAlteracao = product.AlteracaoPreco.Time
		}
	}
	// TODO: Send the updates to Oscar
	hash, err := calculateCurrentHash(queryEngine)
	if err != nil {
		log.Println(err)
		return
	}
	queryEngine.UpdatePriceWatcher(context.TODO(), hash)
}

func notifySimultaneousPriceChanges(queryEngine *db.Queries, product db.GetProductsPricesRow, connpool *pgxpool.Pool, lastCodigoUnidade int) {
	_, err := queryEngine.GetNotificationEvent(context.TODO(), db.GetNotificationEventParams{
		EventType:  "ALTERACOES_SIMULTANEAS",
		CodigoItem: product.CodigoItem,
	})
	// TODO: I'm not sure this is the error that is returned
	if err == pgx.ErrNoRows {
		tx, err := connpool.Begin(context.TODO())
		if err != nil {
			log.Println(err)
			return
		}
		defer tx.Rollback(context.TODO())
		err = queryEngine.WithTx(tx).RegisterNotificationEvent(
			context.TODO(),
			db.RegisterNotificationEventParams{
				EventType:  "ALTERACOES_SIMULTANEAS",
				CodigoItem: product.CodigoItem,
			},
		)
		if err != nil {
			log.Println(err)
			tx.Rollback(context.TODO())
			return
		}
		err = smtp.SendMail(
			EMAIL_SMTP_SERVER_ADDR,
			EMAIL_AUTH, EMAIL_FROM_FORMATTED,
			[]string{EMAIL_ADMIN_ADDRESS},
			[]byte(fmt.Sprintf(
				("O produto com o código %d (ou %f) no Acesse tem"+
					"duas alterações de preço recentes para as "+
					"unidades com código %d e %d."),
				int(product.CodigoItem),
				product.CodigoItem,
				product.CodigoUnidade,
				lastCodigoUnidade)),
		)
		if err != nil {
			log.Println(err)
			tx.Rollback(context.TODO())
			return
		}

	}
}

func main() {
	closing := make(chan interface{})
	updateQueue := make(chan float64)
	ctx := context.TODO()
	connConfig, err := pgx.ParseConfig(os.Getenv("POSTGRES_URL"))
	if err != nil {
		log.Fatal(err)
	}
	connpool, err := pgxpool.NewWithConfig(
		ctx,
		&pgxpool.Config{
			ConnConfig:      connConfig,
			MaxConnIdleTime: 30 * time.Second,
			MaxConns:        10,
		},
	)
	if err != nil {
		log.Fatal(err)
	}
	defer connpool.Close()

	queryEngine := db.New(connpool)

	go UpdaterGoroutine(connpool, queryEngine, closing, updateQueue)

	for {
		select {
		case <-closing:
			fmt.Println("Closing gracefully...")
			return
		default:
			fmt.Println("Checking for updates...")
		}
		if isThereNewUpdate(queryEngine, closing) {
			notifyOfUpdate(queryEngine, updateQueue, closing)
		}
		queryEngine.ClearNotificationEvents(ctx, HOURS_BETWEEN_NOTIFICATIONS)
		time.Sleep(15 * time.Minute)
	}

}
