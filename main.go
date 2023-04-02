package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/patrickmn/go-cache"
	log "github.com/sirupsen/logrus"
	flag "github.com/spf13/pflag"

	"github.com/gograz/gograz-meetup/meetupcom"
)

type server struct {
	client  *meetupcom.Client
	urlName string
	cache   *cache.Cache
}

type attendee struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ThumbLink string `json:"thumbLink"`
	PhotoLink string `json:"photoLink"`
	Guests    int64  `json:"guests"`
}

type rsvps struct {
	Yes []attendee `json:"yes"`
	No  []attendee `json:"no"`
}

//go:embed templates/*
var tmplFS embed.FS

var tmpl *template.Template

func init() {
	tmpl = template.Must(template.New("root").ParseFS(tmplFS, "templates/*.html"))
}

func convertRSVPs(in meetupcom.RSVPsResponse) rsvps {
	out := rsvps{
		Yes: make([]attendee, 0, 2),
		No:  make([]attendee, 0, 2),
	}
	for _, item := range in {
		m := attendee{
			ID:        item.Member.ID,
			Name:      item.Member.Name,
			PhotoLink: item.Member.Photo.PhotoLink,
			ThumbLink: item.Member.Photo.ThumbLink,
		}
		if item.Response == "YES" {
			out.Yes = append(out.Yes, m)
		} else if item.Response == "NO" {
			out.No = append(out.No, m)
		}
	}
	return out
}

func (s *server) handleGetRSVPs(w http.ResponseWriter, r *http.Request) {
	eventID := chi.URLParam(r, "eventID")
	cacheKey := fmt.Sprintf("rsvps:%s", eventID)
	var rsvps *meetupcom.RSVPsResponse
	var err error

	cached, found := s.cache.Get(cacheKey)
	if found {
		t := cached.(meetupcom.RSVPsResponse)
		rsvps = &t
	} else {
		ctx, cancelFunc := context.WithTimeout(context.Background(), time.Second*2)
		defer cancelFunc()
		rsvps, err = s.client.GetRSVPs(ctx, eventID, s.urlName)
		if err != nil {
			log.WithError(err).Errorf("Failed to fetch RSVPs for %s", eventID)
			http.Error(w, "Failed to fetch RSVPs from backend", http.StatusInternalServerError)
			return
		}
		s.cache.Set(cacheKey, *rsvps, 0)
	}
	output := convertRSVPs(*rsvps)

	if r.Header.Get("hx-request") == "true" {
		w.Header().Set("Content-Type", "text/html")
		if err = tmpl.ExecuteTemplate(w, "rsvps.html", output); err != nil {
			log.WithError(err).Errorf("Failed to render output")
			http.Error(w, "Failed to render output", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "text/json")
	_ = json.NewEncoder(w).Encode(output)
}

func main() {
	var addr string
	var urlName string
	var allowedOrigins []string

	flag.StringVar(&addr, "addr", "127.0.0.1:8080", "Address to listen on")
	flag.StringVar(&urlName, "url-name", "Graz-Open-Source-Meetup", "URL name of the meetup group on meetup.com")
	flag.StringSliceVar(&allowedOrigins, "allowed-origins", []string{"http://localhost:1313", "https://gograz.org"}, "Allowed origin hosts")
	flag.Parse()

	ch := cache.New(5*time.Minute, 10*time.Minute)

	s := server{
		client:  meetupcom.NewClient(meetupcom.ClientOptions{}),
		urlName: urlName,
		cache:   ch,
	}

	router := chi.NewRouter()
	router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   allowedOrigins,
		AllowedMethods:   []string{http.MethodGet},
		AllowCredentials: true,
		AllowedHeaders:   []string{"hx-current-url", "hx-request", "content-type", "accept"},
	}))
	router.Get("/{eventID}/rsvps", s.handleGetRSVPs)
	router.Get("/alive", func(w http.ResponseWriter, r *http.Request) {
	})
	log.Infof("Starting HTTPD on %s", addr)
	if err := http.ListenAndServe(addr, router); err != nil {
		log.Fatalf("Listener existed: %s", err.Error())
	}
}
