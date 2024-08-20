CREATE TABLE "item_preco" (
	"codigo_item" DOUBLE PRECISION NOT NULL,
	"codigo_unidade" INTEGER NOT NULL,
	"codigo_prazo" INTEGER NOT NULL,
	"codigo_comissao" INTEGER NULL DEFAULT NULL,
	"preco" NUMERIC(18,4) NOT NULL DEFAULT 0,
	"permite_desconto" INTEGER NOT NULL DEFAULT (-1),
	"desconto_maximo" NUMERIC(4,2) NOT NULL DEFAULT 0,
	"alteracao_preco" DATE NOT NULL DEFAULT ('now'::text)::date,
	"desconto_maximo_prog" NUMERIC(4,2) NOT NULL DEFAULT 0,
	"debito_empresa_prog" NUMERIC(4,2) NOT NULL DEFAULT 0,
	"credito_vendedor_prog" NUMERIC(4,2) NOT NULL DEFAULT 0,
	"debito_vendedor_prog" NUMERIC(4,2) NOT NULL DEFAULT 0,
	"numero_alteracao" INTEGER NOT NULL DEFAULT 0,
	"flag_delivery" INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE "vn_last_notification_event" (
	"event_type" VARCHAR(255) NOT NULL,
	"codigo_item" DOUBLE PRECISION NOT NULL,
	"date_sent" TIMESTAMPTZ NOT NULL,
	PRIMARY KEY ("event_type", "codigo_item")
);

CREATE TABLE "vn_price_update_watcher" (
	"last_update" TIMESTAMPTZ NOT NULL DEFAULT ('now'::text)::timestamptz,
	"prices_hash" VARCHAR(255) NOT NULL DEFAULT '0',
	PRIMARY KEY ("last_update")
);