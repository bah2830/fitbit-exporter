BEGIN;

CREATE TABLE IF NOT EXISTS token (
    access_token    TEXT NOT NULL,
    token_type      TEXT NOT NULL,
    refresh_token   TEXT NOT NULL,
    expiration      DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS heart_data (
    date    DATETIME PRIMARY KEY,
    value   INT NOT NULL
);

CREATE TABLE IF NOT EXISTS heart_rest (
    date    DATETIME PRIMARY KEY,
    value   INT NOT NULL
);

CREATE TABLE IF NOT EXISTS heart_zone (
    date        DATETIME NOT NULL,
    type        VARCHAR(150) NOT NULL,
    minutes     INT NOT NULL,
    calories    INT NOT NULL,

    PRIMARY KEY (date, type)
);

CREATE INDEX heart_data_value ON heart_data (value);
CREATE INDEX heart_rest_value ON heart_rest (value);
CREATE INDEX heart_zone_type ON heart_zone (type);

COMMIT;