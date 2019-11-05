package database

import (
	"database/sql"
	"fmt"

	"github.com/bah2830/fitbit-exporter/pkg/config"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/mysql"
)

const DateTimeFormat = "2006-01-02 15:04:05"

type Database struct {
	db  *sql.DB
	cfg *config.Config
}

func Open(cfg *config.Config) (*Database, error) {
	connectionString := fmt.Sprintf(
		"%s:%s@tcp(%s)/%s?multiStatements=true",
		cfg.Database.Username,
		cfg.Database.Password,
		cfg.Database.Host,
		cfg.Database.Database,
	)
	db, err := sql.Open("mysql", connectionString)
	if err != nil {
		return nil, err
	}
	db.SetMaxIdleConns(0)

	return &Database{db: db, cfg: cfg}, nil
}

func (db *Database) Migrate() error {
	driver, err := mysql.WithInstance(db.db, &mysql.Config{DatabaseName: db.cfg.Database.Database})
	if err != nil {
		return err
	}

	m, err := migrate.NewWithDatabaseInstance("file://migrations", "mysql", driver)
	if err != nil {
		return err
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}

	return nil
}

func (db *Database) GetDB() *sql.DB {
	return db.db
}
