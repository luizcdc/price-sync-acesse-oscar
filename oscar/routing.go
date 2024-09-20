package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/julienschmidt/httprouter"
	"github.com/luizcdc/sync-acesse-oscar/oscar/db"
)

type Auth struct {
	handler *httprouter.Router
}

// ServeHTTP is implements the http.Handler interface for the Auth struct, checking the
// Authorization header for the API_KEY before serving the request.
func (a *Auth) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("REQUEST — %s %s — Remote address: %s — host %s — headers %v\n", r.Method, r.URL.Path, r.RemoteAddr, r.Host, r.Header)
	if API_KEY != "" && r.Header.Get("Authorization") != fmt.Sprintf("Bearer %s", API_KEY) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	a.handler.ServeHTTP(w, r)
}

// createAuthSubRouter initializes an auth-only subrouter, setting up routes and handlers.
func CreateBearerAuthRouter() *Auth {
	underlyingRouter := httprouter.New()

	// Roteia normalmente

	AuthRouter := &Auth{underlyingRouter}

	underlyingRouter.GET("/price-update/catalogue-product", replyWithProductsList)
	underlyingRouter.POST("/price-update/catalogue-product", receivePriceUpdates)

	return AuthRouter
}

func StartRouter() {
	router := CreateBearerAuthRouter()

	SERVER_PORT = 8080

	log.Printf("Starting server on port %d\n", SERVER_PORT)

	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%v", SERVER_PORT), router))
}

func readBody(body io.ReadCloser) []byte {
	buf := new(bytes.Buffer)
	buf.ReadFrom(body)
	return buf.Bytes()
}

func replyWithProductsList(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	log.Printf("Received notice of available update and request for products-to-update list\n")
	ctx := context.Background()
	luStr := r.URL.Query().Get("lastupdate")
	luTime, err := time.Parse("2006-01-02 15:04:05-07", luStr)
	if err != nil {
		log.Printf("Error parsing last update time: %v\nReceived value: '%s'\n", err, luStr)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Invalid last update time format."))
		return
	}
	lastUpdatePgType := pgtype.Timestamptz{Time: luTime, Valid: true}
	partnerSkus, err := queryEngine.GetAllProductsThatNeedUpdate(
		ctx,
		db.GetAllProductsThatNeedUpdateParams{
			PartnerID:   VN_PARTNER_ID,
			DateUpdated: lastUpdatePgType,
		},
	)
	if err != nil {
		log.Printf("Error getting products that need update: %v\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(fmt.Sprintf("Error getting products that need update: %v\n", err)))
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", APPLICATION_JSON)
	if len(partnerSkus) == 0 {
		log.Printf("No products that were updated before %s. Replying with null.\n", luStr)
		w.Write([]byte("null"))
		return
	}

	var buffer bytes.Buffer
	buffer.WriteString("\"")
	for i, sku := range partnerSkus {
		if i > 0 {
			buffer.WriteString(",")
		}
		buffer.WriteString(sku)
	}
	buffer.WriteString("\"")
	w.Write(buffer.Bytes())
	log.Printf("Replied with %d products: %s\n", len(partnerSkus), buffer.String())

}

func receivePriceUpdates(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	var products []struct {
		Codigo float64 `json:"codigo"`
		Preco  float64 `json:"preco"`
	}
	defer r.Body.Close()
	fmt.Println("Received Price Update")
	bodyContents := readBody(r.Body)
	log.Printf("Received body: `%s`\n", bodyContents)
	if err := json.Unmarshal(bodyContents, &products); err != nil {
		log.Printf("Error reading request body: '%v'\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Error reading request body."))
		return
	}
	productsToUpdate := make([]db.UpdateProductPriceParams, len(products))
	for _, product := range products {
		productsToUpdate = append(productsToUpdate, db.UpdateProductPriceParams{
			Price:      product.Preco,
			PartnerID:  VN_PARTNER_ID,
			PartnerSku: fmt.Sprintf("%.0f", product.Codigo),
		})
	}
	res := queryEngine.UpdateProductPrice(
		context.TODO(),
		productsToUpdate,
	)
	var errors bool
	res.Exec(
		func(i int, err error) {
			if err != nil {
				log.Printf("Error updating product price on batch item %d: %v\n", i, err)
				errors = true
			}
		})
	if !errors {
		log.Printf("Updated %d products\n", len(products))
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusInternalServerError)
	}
}
