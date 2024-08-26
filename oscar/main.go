package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/julienschmidt/httprouter"
	"github.com/luizcdc/sync-acesse-oscar/oscar/db"
)

var API_KEY string
var VN_PARTNER_ID int64
const APPLICATION_JSON = "application/json"

var SERVER_PORT uint16

// ###################################### AUTH MIDDLEWARE #######################################
type Auth struct {
	handler httprouter.Router
}

// ServeHTTP is implements the http.Handler interface for the Auth struct, checking the
// Authorization header for the API_KEY before serving the request.
func (a *Auth) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if API_KEY != "" && r.Header.Get("Authorization") != fmt.Sprintf("Bearer %s", API_KEY) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	a.handler.ServeHTTP(w, r)
}

// createAuthSubRouter initializes an auth-only subrouter, setting up routes and handlers.
func CreateAuthSubRouter() *Auth {
	requireAuthRouter := httprouter.New()
	// requireAuthRouter.POST(API_ROOT+"set/:path", SetSpecificRedirect)
	// requireAuthRouter.POST(API_ROOT+"set", SetRandomRedirect)
	// requireAuthRouter.DELETE(API_ROOT+"del/:path", DelRedirect)
	// requireAuthRouter.GET(API_ROOT+"stats/urlcount", GetTotalSetRedirects)
	// requireAuthRouter.GET(API_ROOT+"stats/redirectcount", GetTotalServedRedirects)
	AuthSubRouter := &Auth{*requireAuthRouter}
	return AuthSubRouter
}
// ################################### END AUTH MIDDLEWARE #####################################

// ###################################   DEFINING ROUTES  #####################################
func DefineRoutes(AuthSubRouter *Auth) *httprouter.Router {
	router := httprouter.New()

	// router.Handler(http.MethodGet, "/:redirectpath/*any", AuthSubRouter)
	// router.Handler(http.MethodPost, API_ROOT+"*any", AuthSubRouter)
	// router.Handler(http.MethodDelete, API_ROOT+"*any", AuthSubRouter)
	// router.Handler(http.MethodPut, API_ROOT+"*any", AuthSubRouter)

	// router.GET("/:redirectpath", Redirect)
	return router
}
// ################################# END DEFINING ROUTES ######################################

func replyWithProductsList(queryEngine *db.Queries, lastUpdate pgtype.Timestamptz){
	ctx := context.Background()
	partnerSkus, err := queryEngine.GetAllProductsThatNeedUpdate(
		ctx,
		db.GetAllProductsThatNeedUpdateParams{
			PartnerID: VN_PARTNER_ID,
			DateUpdated: lastUpdate,
		},
	)
	if err != nil {
		panic("HA")
	}
}

func main() {
	ctx := context.Background()
	d, err := pgxpool.New(ctx, fmt.Sprintf("user=%v dbname=%v sslmode=disable host=localhost port=%v"))
	if err != nil {
		return
	}
	queryEngine := db.New(d)
	
	for i := range 5 {
		fmt.Println(i)

	}
}