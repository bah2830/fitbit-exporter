package exporter

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"log"
	"math"
	"net/http"
	"strings"
	"text/template"
	"time"

	"github.com/bah2830/fitbit-exporter/pkg/fitbit"
)

type indexData struct {
	BackfillerRunning bool             `json:"backfillerRunning"`
	BackfillerLastRun time.Time        `json:"backfillerLastRun,omitempty"`
	Last7DaysZones    *zones           `json:"last7DaysZones,omitempty"`
	Last30DaysZones   *zones           `json:"last30DaysZones,omitempty"`
	PersonalRecords   *personalRecords `json:"personalRecords,omitempty"`
	CurrentDay        *currentDay      `json:"currentDay,omitempty"`
}

type currentDay struct {
	Resting    int                `json:"resting,omitempty"`
	High       *fitbit.HeartData  `json:"high,omitempty"`
	Low        *fitbit.HeartData  `json:"low,omitempty"`
	Zones      *zones             `json:"zones,omitempty"`
	HeartRates []fitbit.HeartData `json:"heartRates,omitempty"`
}

type personalRecords struct {
	Top10HeartRates    []fitbit.HeartData `json:"top10HeartRates,omitempty"`
	Bottom10HeartRates []fitbit.HeartData `json:"bottom10HeartRates,omitempty"`
	MinResting         *fitbit.HeartData  `json:"minResting,omitempty"`
	MaxResting         *fitbit.HeartData  `json:"maxResting,omitempty"`

	MostOutOfRange *zone `json:"mostOutOfRange,omitempty"`
	MostFatBurn    *zone `json:"mostFatBurn,omitempty"`
	MostCardio     *zone `json:"mostCardio,omitempty"`
	MostPeak       *zone `json:"mostPeak,omitempty"`
}

type zones struct {
	OutOfRange zone `json:"outOfRange,omitempty"`
	FatBurn    zone `json:"fatBurn,omitempty"`
	Cardio     zone `json:"cardio,omitempty"`
	Peak       zone `json:"peak,omitempty"`
}

type zone struct {
	Date     string  `json:"date,omitempty"`
	Percent  float64 `json:"percent,omitempty"`
	Minutes  int     `json:"minutes,omitempty"`
	Calories float64 `json:"calories,omitempty"`
}

type frontendErr struct {
	Error string
}

type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
}

func (w gzipResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

func writeErr(w http.ResponseWriter, statueCode int, err error) {
	body, _ := json.Marshal(frontendErr{Error: err.Error()})
	w.WriteHeader(statueCode)
	w.Write(body)
}

func (e *Exporter) startFrontend() error {
	http.HandleFunc("/", gzipHandler(e.indexHandler))
	http.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {})
	http.HandleFunc("/login", e.client.LoginHandler)
	http.HandleFunc("/callback", e.client.CallbackHandler)
	http.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("frontend/assets"))))

	log.Println("listening on localhost:3000")
	go http.ListenAndServe(":3000", nil)
	return nil
}

func gzipHandler(fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			fn(w, r)
			return
		}
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Type", "text/html")
		gz := gzip.NewWriter(w)
		defer gz.Close()
		gzr := gzipResponseWriter{Writer: gz, ResponseWriter: w}
		fn(gzr, r)
	}
}

func (e *Exporter) indexHandler(w http.ResponseWriter, r *http.Request) {
	if e.user == nil {
		writeErr(w, http.StatusForbidden, errors.New("login required"))
		return
	}

	top10Hr, err := e.client.GetNHeartRates(true, 10)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	bottom10Hr, err := e.client.GetNHeartRates(false, 10)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	topResting, err := e.client.GetResting(true)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	bottomResting, err := e.client.GetResting(false)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	currentResting, err := e.client.GetCurrentResting()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	currentData, err := e.client.GetCurrentDaysData()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	currentHigh, err := e.client.GetCurrentDayLimit(true)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	currentLow, err := e.client.GetCurrentDayLimit(false)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	currentDayZones, err := e.client.GetCurrentDayZones()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	last7DaysZones, err := e.client.GetZonesByDate(time.Now().Add(-7*24*time.Hour), time.Now())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	last30DaysZones, err := e.client.GetZonesByDate(time.Now().Add(-30*24*time.Hour), time.Now())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	maxZones, err := e.client.GetMaxZones()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}

	data := indexData{
		BackfillerRunning: e.backfillRunning,
		BackfillerLastRun: e.backfillLastRun,
		Last7DaysZones:    zonesToPercentages(last7DaysZones),
		Last30DaysZones:   zonesToPercentages(last30DaysZones),
		PersonalRecords: &personalRecords{
			Top10HeartRates:    top10Hr,
			Bottom10HeartRates: bottom10Hr,
			MaxResting:         topResting,
			MinResting:         bottomResting,
			MostOutOfRange:     fitbitZoneToZone(maxZones["Out of Range"]),
			MostFatBurn:        fitbitZoneToZone(maxZones["Fat Burn"]),
			MostCardio:         fitbitZoneToZone(maxZones["Cardio"]),
			MostPeak:           fitbitZoneToZone(maxZones["Peak"]),
		},
		CurrentDay: &currentDay{
			Resting:    currentResting,
			HeartRates: currentData,
			High:       currentHigh,
			Low:        currentLow,
			Zones:      zonesToPercentages(currentDayZones),
		},
	}

	templateFuncs := template.FuncMap{
		"json": func(v interface{}) string {
			a, _ := json.MarshalIndent(v, "", "  ")
			return string(a)
		},
		"duration": func(in int) time.Duration {
			return time.Duration(in) * time.Second
		},
	}

	tmpl, err := template.New("index.template.html").
		Funcs(templateFuncs).
		ParseFiles("frontend/templates/index.template.html")

	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}

	if err := tmpl.Execute(w, data); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
}

func zonesToPercentages(inZones []fitbit.HeartRateZone) *zones {
	var total int
	var rest, fat, cardio, peak zone
	for _, z := range inZones {
		total += z.Minutes
		switch strings.ToLower(z.Name) {
		case "out of range":
			rest.Minutes += z.Minutes
			rest.Calories += z.CaloriesOut
		case "fat burn":
			fat.Minutes += z.Minutes
			fat.Calories += z.CaloriesOut
		case "cardio":
			cardio.Minutes += z.Minutes
			cardio.Calories += z.CaloriesOut
		case "peak":
			peak.Minutes += z.Minutes
			peak.Calories += z.CaloriesOut
		}
	}

	var percent = func(input int) float64 {
		return math.Round(float64(input) / float64(total) * 100.0)
	}

	rest.Percent = percent(rest.Minutes)
	fat.Percent = percent(fat.Minutes)
	cardio.Percent = percent(cardio.Minutes)
	peak.Percent = percent(peak.Minutes)

	return &zones{
		OutOfRange: rest,
		FatBurn:    fat,
		Cardio:     cardio,
		Peak:       peak,
	}
}

func fitbitZoneToZone(z fitbit.HeartRateZone) *zone {
	return &zone{
		Date:     z.Date,
		Minutes:  z.Minutes,
		Calories: z.CaloriesOut,
	}
}
