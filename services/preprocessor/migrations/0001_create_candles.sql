-- services/preprocessor/migrations/0001_create_candles.sql

CREATE TABLE IF NOT EXISTS candles (
  time TIMESTAMPTZ NOT NULL,
  symbol TEXT NOT NULL,
  interval TEXT NOT NULL,
  open DOUBLE PRECISION,
  high DOUBLE PRECISION,
  low DOUBLE PRECISION,
  close DOUBLE PRECISION,
  volume DOUBLE PRECISION,
  PRIMARY KEY (symbol, interval, time)
);

SELECT create_hypertable('candles', 'time', if_not_exists => TRUE);
