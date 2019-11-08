package webserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"strings"
	"text/template"
	"time"

	"github.com/bah2830/fitbit-exporter/pkg/fitbit"
	"github.com/gorilla/mux"
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

func (s *Server) userHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	user, ok := vars["user"]
	if !ok || user == "" {
		writeErr(w, http.StatusBadRequest, errors.New("user not given"))
	}

	top10Hr, err := s.client.GetNHeartRates(user, true, 10)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, fmt.Errorf("GetNHeartRates: "+err.Error()))
		return
	}
	bottom10Hr, err := s.client.GetNHeartRates(user, false, 10)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, fmt.Errorf("GetNHeartRates: "+err.Error()))
		return
	}
	topResting, err := s.client.GetResting(user, true)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, fmt.Errorf("GetResting: "+err.Error()))
		return
	}
	bottomResting, err := s.client.GetResting(user, false)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, fmt.Errorf("GetResting: "+err.Error()))
		return
	}
	currentResting, err := s.client.GetCurrentResting(user)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, fmt.Errorf("GetCurrentResting: "+err.Error()))
		return
	}
	currentData, err := s.client.GetCurrentDaysData(user)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, fmt.Errorf("GetCurrentDaysData: "+err.Error()))
		return
	}
	currentHigh, err := s.client.GetCurrentDayLimit(user, true)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, fmt.Errorf("GetCurrentDayLimit: "+err.Error()))
		return
	}
	currentLow, err := s.client.GetCurrentDayLimit(user, false)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, fmt.Errorf("GetCurrentDayLimit: "+err.Error()))
		return
	}
	currentDayZones, err := s.client.GetCurrentDayZones(user)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, fmt.Errorf("GetCurrentDayZones: "+err.Error()))
		return
	}
	last7DaysZones, err := s.client.GetZonesByDate(user, time.Now().Add(-7*24*time.Hour), time.Now())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, fmt.Errorf("GetZonesByDate: "+err.Error()))
		return
	}
	last30DaysZones, err := s.client.GetZonesByDate(user, time.Now().Add(-30*24*time.Hour), time.Now())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, fmt.Errorf("GetZonesByDate: "+err.Error()))
		return
	}
	maxZones, err := s.client.GetMaxZones(user)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, fmt.Errorf("GetMaxZones: "+err.Error()))
		return
	}

	data := indexData{
		BackfillerRunning: s.exporter.BackfillRunning,
		BackfillerLastRun: s.exporter.BackfillLastRun,
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

	tmpl, err := template.New("user.template.html").
		Funcs(template.FuncMap{
			"json": func(v interface{}) string {
				a, err := json.MarshalIndent(v, "", "  ")
				if err != nil {
					log.Println("error marshaling json output: " + err.Error())
				}
				return string(a)
			},
			"duration": func(in int) time.Duration {
				return time.Duration(in) * time.Second
			},
		}).
		ParseFiles("frontend/templates/user.template.html")

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
		if total == 0 {
			return 0.0
		}
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
