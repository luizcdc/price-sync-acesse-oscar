CREATE TABLE "partner_stockrecord" (
	"id" BIGINT NOT NULL,
	"partner_sku" VARCHAR(128) NOT NULL,
	"price_currency" VARCHAR(12) NOT NULL,
	"price" NUMERIC(12,2) NULL DEFAULT NULL,
	"num_in_stock" INTEGER NULL DEFAULT NULL,
	"num_allocated" INTEGER NULL DEFAULT NULL,
	"low_stock_threshold" INTEGER NULL DEFAULT NULL,
	"date_created" TIMESTAMPTZ NOT NULL,
	"date_updated" TIMESTAMPTZ NOT NULL,
	"partner_id" BIGINT NOT NULL,
	"product_id" BIGINT NOT NULL,
	PRIMARY KEY ("id")
);