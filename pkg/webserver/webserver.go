package webserver

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/bah2830/fitbit-exporter/pkg/config"
	"github.com/bah2830/fitbit-exporter/pkg/exporter"
	"github.com/bah2830/fitbit-exporter/pkg/fitbit"
	"github.com/gorilla/sessions"
)

type frontendErr struct {
	Error string
}

type Server struct {
	client   *fitbit.Client
	cfg      *config.Config
	exporter *exporter.Exporter
	cookies  *sessions.CookieStore
}

func New(cfg *config.Config, client *fitbit.Client, exporter *exporter.Exporter) *Server {
	return &Server{
		cfg:      cfg,
		client:   client,
		exporter: exporter,
		cookies:  sessions.NewCookieStore([]byte(cfg.WebFrontend.SessionKey)),
	}
}

func (s *Server) Start() error {
	http.HandleFunc("/", gzipHandler(s.indexHandler))
	http.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {})
	http.HandleFunc("/login", s.client.LoginHandler)
	http.HandleFunc("/callback", s.client.CallbackHandler)
	http.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("frontend/assets"))))

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
