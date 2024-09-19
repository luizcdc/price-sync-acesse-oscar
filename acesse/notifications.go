package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luizcdc/sync-acesse-oscar/acesse/db"
)

// notifyAdmin sends an email notification to the admin email address and
// registers the notification event in the database.
func notifyAdmin(queryEngine *db.Queries, connpool *pgxpool.Pool, eventType string, codigoItem float64, message string) {
	_, err := queryEngine.GetNotificationEvent(context.TODO(), db.GetNotificationEventParams{
		EventType:  eventType,
		CodigoItem: codigoItem,
	})
	if err == nil || !strings.Contains(err.Error(), "no rows") {
		log.Printf("Notification already sent within the last %v hours.", HOURS_BETWEEN_NOTIFICATIONS)
		return
	}
	tx, err := connpool.Begin(context.TODO())
	if err != nil {
		log.Println(err)
		return
	}
	defer tx.Rollback(context.TODO())
	err = queryEngine.WithTx(tx).RegisterNotificationEvent(
		context.TODO(),
		db.RegisterNotificationEventParams{
			EventType:  eventType,
			CodigoItem: codigoItem,
		},
	)
	if err != nil {
		log.Println(err)
		tx.Rollback(context.TODO())
		return
	}
	log.Printf("Notification event %s registered.", eventType)
	log.Println("Sending email notification...")
	err = smtp.SendMail(
		EMAIL_SMTP_SERVER_ADDR,
		EMAIL_AUTH, EMAIL_FROM,
		[]string{EMAIL_ADMIN_ADDRESS},
		[]byte(
			fmt.Sprintf(
				"From: %s\r\n"+
					"To: Admin <%s>\r\n"+
					"Subject: Integração Oscar Acesse - %s\r\n\r\n%s",
				EMAIL_FROM_FORMATTED,
				EMAIL_ADMIN_ADDRESS,
				eventType,
				message,
			)),
	)
	if err != nil {
		log.Printf("Error sending %s email notification: %s", eventType, err.Error())
		tx.Rollback(context.TODO())
		return
	}
	tx.Commit(context.TODO())
	log.Printf("Email notification sent for event %s", eventType)
}

// NotifyErrorResponse sends an email notifying of an error response 
// from the Oscar server.
func NotifyErrorResponse(queryEngine *db.Queries, connpool *pgxpool.Pool, resp *http.Response) {
	message := fmt.Sprintf("O servidor do Oscar respondeu com um erro: %s.\nBody: %s", resp.Status, readBody(resp))
	notifyAdmin(queryEngine, connpool, "OSCAR_NOT_OK_RESPONSE", -1.0, message)
}

// NotifyOfNoConnection sends an email notifying of the inability to connect to the Oscar server.
func NotifyOfNoConnection(queryEngine *db.Queries, connpool *pgxpool.Pool) {
	message := "Não foi possível se conectar ao servidor do Oscar para enviar as atualizações de preço."
	notifyAdmin(queryEngine, connpool, "NO_CONNECTION", -1.0, message)
}

// NotifySimultaneousPriceChanges sends an email notifying of simultaneous price changes
// in different measuring units for the same product, which impossibilitates knowing which is 
// the correct price.
func NotifySimultaneousPriceChanges(queryEngine *db.Queries, product db.GetProductsPricesRow, connpool *pgxpool.Pool, lastCodigoUnidade int) {
	message := fmt.Sprintf(
		("O produto com o código %d (ou %f) no Acesse tem " +
			"duas alterações de preço recentes para as " +
			"unidades com código %d e %d."),
		int(product.CodigoItem),
		product.CodigoItem,
		product.CodigoUnidade,
		lastCodigoUnidade,
	)
	notifyAdmin(queryEngine, connpool, "ALTERACOES_SIMULTANEAS", product.CodigoItem, message)
}