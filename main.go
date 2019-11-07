package main

import (
	"flag"
	"os"
	"os/signal"

	"github.com/bah2830/fitbit-exporter/pkg/config"
	"github.com/bah2830/fitbit-exporter/pkg/database"
	"github.com/bah2830/fitbit-exporter/pkg/exporter"
	"github.com/bah2830/fitbit-exporter/pkg/fitbit"
	"github.com/bah2830/fitbit-exporter/pkg/webserver"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

var configPath = flag.String("config.file", "config.example.yaml", "Path to config file")

func main() {
	flag.Parse()

	conf, err := config.LoadConfig(*configPath)
	if err != nil {
		panic(err)
	}

	db, err := database.Open(conf)
	if err != nil {
		panic(err)
	}

	if err := db.Migrate(); err != nil {
		panic(err)
	}

	client, err := fitbit.NewClient(db, conf.Fitbit.ClientID, conf.Fitbit.ClientSecret)
	if err != nil {
		panic(err)
	}

	exporter := exporter.New(conf, client, db)
	go exporter.Start()
	defer exporter.Stop()

	server := webserver.New(conf, client, exporter)
	if err := server.Start(); err != nil {
		panic(err)
	}
	defer server.Stop()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
}
