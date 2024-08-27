package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luizcdc/sync-acesse-oscar/acesse/db"
)

var API_KEY string
var VN_PARTNER_ID int64
var CODIGO_PRAZO int
const APPLICATION_JSON = "application/json"

var SERVER_PORT uint16

// calculateCurrentHash calculates the hash of all products' prices
// and returns it as a string. 
func calculateCurrentHash(queryEngine *db.Queries) (string, error) {
	products, err := queryEngine.GetAllProducts(context.Background(), CODIGO_PRAZO)
	if err != nil {
		return "", err
	}
	var allProductsString bytes.Buffer
	for _, product := range products {
			allProductsString.WriteString(fmt.Sprintf("%v%v",product.CodigoItem, product.Preco))
	}
	finalHash := fmt.Sprintf("%x", sha256.Sum256(allProductsString.Bytes()))
	return finalHash, nil
}

// isThereNewUpdate compares the last stored hash and the calculated current hash
// of all products' prices, signaling whether there has been any update or not.
func isThereNewUpdate(queryEngine *db.Queries, closingChannel chan interface{}) bool {
	watcher, err := queryEngine.GetPriceWatcher(context.Background())
	if err != nil {
		defer closeChannel(closingChannel)
		panic(err)
	}
	
	currentHash, err := calculateCurrentHash(queryEngine)
	if err != nil {
		defer closeChannel(closingChannel)
		panic(err)
	}

	return watcher.PricesHash != currentHash
}

func readBody(resp *http.Response) string {
	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)
	return buf.String()
}

// notifyUpdate notifies Oscar that there has been an update of the prices.
// If the update is acknowledged but refused, the last update time and hash are updated.
// If the update is acknowledged and accepted, it sends the product codes provided in the answer
// to the updateQueue channel.
func notifyUpdate(queryEngine *db.Queries, updateQueue chan float64, closing chan interface{}) {
	// TODO implement notifyUpdate
	client := http.Client{
		Timeout: 15 * time.Second,
	}
	mostRecentUpdate, err := queryEngine.GetMostRecentUpdate(context.Background())
	if err != nil {
		defer closeChannel(closing)
		panic(err)
	}
	// TODO: env variable timezone, -03:00
	
	// We send the most recent update as 23:59:59 of the recorded day because the database
	// only stores the date.
	// TODO: integrate credentials
	lastUpdateTimestamp := url.QueryEscape(fmt.Sprintf("%sT23:59:59%s", mostRecentUpdate.Time.Format("2006-01-02"), localTimezone))
	resp, err := client.Get(fmt.Sprintf("http://localhost:8080/api/integration/price-update/catalogue-product?lastupdate=%s&key=%s", lastUpdateTimestamp, url.QueryEscape(API_KEY)))
	if err != nil {
		defer closeChannel(closing)
		panic(err)
	}
	defer resp.Body.Close()

	
	text := strings.Trim(readBody(resp), "\"")

	if text == "null" {
		hash, err := calculateCurrentHash(queryEngine)
		if err != nil {
			defer closeChannel(closing)
			panic(err)
		}
		queryEngine.UpdatePriceWatcher(context.Background(), hash)
		return
	}
	
	for _, codigo := range strings.Split(text, ",") {
		codigoFloat, err := strconv.ParseFloat(codigo, 64)
		if err != nil {
			defer closeChannel(closing)
			panic(err)
		}
		updateQueue <- codigoFloat
	}
	updateQueue <- -1.0 // Signals that the current update has finished
}

func closeChannel(toClose chan interface{}) {
	select {
	case <-toClose:
		return
	default:
		close(toClose)
	}
}

func sendUpdates(queryEngine *db.Queries, closing chan interface{}, updateQueue chan float64) {
	codigos := make([]float64, 0, 500)
	for {
		select {
		case <-closing:
			return
		case codigo := <-updateQueue:
			if codigo == -1.0 {
				// SEND ALL UPDATES AS JSON TO OSCAR
				codigos := make([]float64, 0, 500)
				// Query the database with codigos
				
				// Check for duplicate codigo_item
				// If there are duplicates, choose the one with the most recent alteracao_preco
				// If there are two with the same alteracao_preco, send a warning to the admin through email

				// Send the updates to Oscar

				// Update the last update time and hash
				continue
			}
			codigos = append(codigos, codigo)
		}
	}
}

func main() {
	closing := make(chan interface{})
	updateQueue := make(chan float64)
	ctx := context.Background()
	// TODO add connstring
	connConfig, err := pgx.ParseConfig("")
	connpool, err := pgxpool.NewWithConfig(
		ctx,
		&pgxpool.Config{
			ConnConfig: connConfig,
			MaxConnIdleTime: 30 * time.Second,
			MaxConns:        10,
		},
	)
	if err != nil {
		panic(err)
	}
	defer connpool.Close()
	
	queryEngine := db.New(connpool)

	go sendUpdates(queryEngine, closing, updateQueue)

	for {
		select {
		case <-closing:
			fmt.Println("Closing gracefully...")
			return
		default:
			fmt.Println("Checking for updates...")
		}
		if isThereNewUpdate(queryEngine, closing){
			notifyUpdate(queryEngine, updateQueue, closing)
		}
		// TODO: parameterize number of hours
		queryEngine.ClearNotificationEvents(ctx, 1)
		time.Sleep(15 * time.Minute)
	}

	
}