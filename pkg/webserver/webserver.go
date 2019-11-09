package webserver

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"text/template"
	"time"

	"github.com/bah2830/fitbit-exporter/pkg/config"
	"github.com/bah2830/fitbit-exporter/pkg/exporter"
	"github.com/bah2830/fitbit-exporter/pkg/fitbit"
	"github.com/gorilla/mux"
)

type frontendErr struct {
	Error string
}

type Server struct {
	client   *fitbit.Client
	cfg      *config.Config
	exporter *exporter.Exporter
}

func New(cfg *config.Config, client *fitbit.Client, exporter *exporter.Exporter) *Server {
	return &Server{
		cfg:      cfg,
		client:   client,
		exporter: exporter,
	}
}

func (s *Server) Start() error {
	r := mux.NewRouter().StrictSlash(true)
	r.HandleFunc("/", gzipHandler(s.indexHandler))
	r.HandleFunc("/login", s.client.LoginHandler)
	r.HandleFunc("/callback", s.client.CallbackHandler)
	r.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {})
	r.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("frontend/assets"))))
	r.HandleFunc("/{user}", gzipHandler(s.userHandler))
	http.Handle("/", r)

	log.Println("listening on " + s.cfg.WebFrontend.Listen)
	go http.ListenAndServe(s.cfg.WebFrontend.Listen, nil)
	return nil
}

func (s *Server) Stop() error {
	return nil
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

func getTemplateFuncs() template.FuncMap {
	return template.FuncMap{
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
	}
}

func (s *Server) indexHandler(w http.ResponseWriter, r *http.Request) {
	t, err := template.New("index.template.html").Funcs(getTemplateFuncs()).ParseFiles("frontend/templates/index.template.html")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}

	t.Execute(w, s.client.Users)
}
