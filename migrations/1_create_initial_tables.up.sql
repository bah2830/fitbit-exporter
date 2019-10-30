BEGIN;

CREATE TABLE IF NOT EXISTS fitbit_token (
    access_token TEXT NOT NULL,
    token_type TEXT NOT NULL,
    refresh_token TEXT NOT NULL,
    expiration DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS heart_data (
    id      INT AUTO_INCREMENT PRIMARY KEY,
    date    DATETIME NOT NULL,
    value   INT NOT NULL
);

CREATE TABLE IF NOT EXISTS heart_rest (
    id      INT AUTO_INCREMENT PRIMARY KEY,
    date    DATETIME NOT NULL,
    value   INT NOT NULL
);

CREATE TABLE IF NOT EXISTS heart_zone (
    id          INT AUTO_INCREMENT PRIMARY KEY,
    date        DATETIME NOT NULL,
    type        TEXT NOT NULL,
    minutes     INT NOT NULL,
    calories    INT NOT NULL
);

CREATE INDEX heart_data_date_value ON heart_data (date, value);
CREATE INDEX heart_rest_date_value ON heart_rest (date, value);
CREATE UNIQUE INDEX heart_zone_date_type ON heart_zone (date, type);

COMMIT;