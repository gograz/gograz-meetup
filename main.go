package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/patrickmn/go-cache"
	"github.com/rs/cors"
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

	cached, found := s.cache.Get(cacheKey)
	if found {
		w.Header().Set("Content-Type", "text/json")
		_ = json.NewEncoder(w).Encode(convertRSVPs(cached.(meetupcom.RSVPsResponse)))
		return
	}

	ctx, cancelFunc := context.WithTimeout(context.Background(), time.Second*2)
	defer cancelFunc()
	rsvps, err := s.client.GetRSVPs(ctx, eventID, s.urlName)
	if err != nil {
		log.WithError(err).Errorf("Failed to fetch RSVPs for %s", eventID)
		http.Error(w, "Failed to fetch RSVPs from backend", http.StatusInternalServerError)
		return
	}
	s.cache.Set(cacheKey, *rsvps, 0)
	w.Header().Set("Content-Type", "text/json")
	_ = json.NewEncoder(w).Encode(convertRSVPs(*rsvps))
}

func main() {
	var addr string
	var urlName string
	var allowedOrigins []string

	flag.StringVar(&addr, "addr", "127.0.0.1:8080", "Address to listen on")
	flag.StringVar(&urlName, "url-name", "Graz-Open-Source-Meetup", "URL name of the meetup group on meetup.com")
	flag.StringArrayVar(&allowedOrigins, "allowed-origins", []string{"http://localhost:1313", "https://gograz.org"}, "Allowed origin hosts")
	flag.Parse()

	ch := cache.New(5*time.Minute, 10*time.Minute)

	s := server{
		client:  meetupcom.NewClient(meetupcom.ClientOptions{}),
		urlName: urlName,
		cache:   ch,
	}

	router := chi.NewRouter()
	c := cors.New(cors.Options{
		AllowedOrigins:   allowedOrigins,
		AllowCredentials: true,
	})
	router.Get("/{eventID}/rsvps", s.handleGetRSVPs)
	router.Get("/alive", func(w http.ResponseWriter, r *http.Request) {
	})
	log.Infof("Starting HTTPD on %s", addr)
	_ = http.ListenAndServe(addr, c.Handler(router))
}
