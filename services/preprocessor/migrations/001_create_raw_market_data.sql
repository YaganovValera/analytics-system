-- services/preprocessor/migrations/001_create_raw_market_data.sql

-- Создаём таблицу для сырых событий.
CREATE TABLE IF NOT EXISTS raw_market_data (
    event_time    TIMESTAMPTZ      NOT NULL,
    symbol        TEXT             NOT NULL,
    price         DOUBLE PRECISION NOT NULL,
    bid_price     DOUBLE PRECISION NOT NULL,
    ask_price     DOUBLE PRECISION NOT NULL,
    volume        DOUBLE PRECISION NOT NULL,
    trade_id      TEXT             NOT NULL,
    PRIMARY KEY (event_time, symbol, trade_id)
);

-- Превращаем её в гипертаблицу TimescaleDB по колонке event_time.
SELECT create_hypertable(
  'raw_market_data',       -- имя таблицы
  'event_time',            -- колонка времени
  if_not_exists => TRUE    -- не упадём, если уже создано
);
