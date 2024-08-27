package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luizcdc/sync-acesse-oscar/acesse/db"
)

var API_KEY string
var VN_PARTNER_ID int64
var CODIGO_PRAZO int
const APPLICATION_JSON = "application/json"

var SERVER_PORT uint16
// 
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

func isThereNewUpdate(queryEngine *db.Queries) bool {
	// query db to get last update time and hash
	watcher, err := queryEngine.GetPriceWatcher(context.Background())
	if err != nil {
		panic("getPriceWatcher")
	}
	
	currentHash, err := calculateCurrentHash(queryEngine)
	if err != nil {
		panic("calcNewHash")
	}

	return watcher.PricesHash == "" || watcher.PricesHash != currentHash
	// if there is a hash, calculate the current hash and compare with it
}


func main() {
	ctx := context.Background()
	// TODO: parameterize db credentials
	d, err := pgxpool.New(ctx, fmt.Sprintf("user=%v dbname=%v sslmode=disable host=localhost port=%v"))
	if err != nil {
		panic("HA")
	}
	queryEngine := db.New(d)

	for {
		if isThereNewUpdate(queryEngine){
			sendUpdate()
		}else{
			time.Sleep(15 * time.Minute)
		}
	}
	
	for i := range 5 {
		fmt.Println(i)

	}
}