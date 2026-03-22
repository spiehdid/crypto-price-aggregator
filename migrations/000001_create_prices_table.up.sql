CREATE TABLE IF NOT EXISTS prices (
    id          BIGSERIAL PRIMARY KEY,
    coin_id     VARCHAR(100) NOT NULL,
    currency    VARCHAR(10)  NOT NULL,
    value       NUMERIC(24, 8) NOT NULL,
    provider    VARCHAR(50)  NOT NULL,
    timestamp   TIMESTAMPTZ  NOT NULL,
    received_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_prices_coin_currency_received ON prices (coin_id, currency, received_at DESC);
CREATE INDEX idx_prices_received_at ON prices (received_at);
