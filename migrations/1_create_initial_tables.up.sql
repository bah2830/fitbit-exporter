BEGIN;

CREATE TABLE IF NOT EXISTS token (
    user_id         VARCHAR(150) PRIMARY KEY,
    user            VARCHAR(150) NOT NULL,
    access_token    TEXT NOT NULL,
    token_type      TEXT NOT NULL,
    refresh_token   TEXT NOT NULL,
    expiration      DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS heart_data (
    user    VARCHAR(150) NOT NULL,
    date    DATETIME NOT NULL,
    value   INT NOT NULL,

    PRIMARY KEY (user, date)
);

CREATE TABLE IF NOT EXISTS heart_rest (
    user    VARCHAR(150) NOT NULL,
    date    DATETIME NOT NULL,
    value   INT NOT NULL,

    PRIMARY KEY (user, date)
);

CREATE TABLE IF NOT EXISTS heart_zone (
    user        VARCHAR(150) NOT NULL,
    date        DATETIME NOT NULL,
    type        VARCHAR(150) NOT NULL,
    minutes     INT NOT NULL,
    calories    INT NOT NULL,

    PRIMARY KEY (user, date, type)
);

CREATE INDEX heart_data_value ON heart_data (value);
CREATE INDEX heart_rest_value ON heart_rest (value);
CREATE INDEX heart_zone_type ON heart_zone (type);

COMMIT;