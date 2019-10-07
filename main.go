package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gograz/gograz-meetup/meetupcom"
	"github.com/gograz/gograz-meetup/pkg/oauth"
	"github.com/patrickmn/go-cache"
	"github.com/pressly/chi"
	"github.com/rs/cors"
	log "github.com/sirupsen/logrus"
	flag "github.com/spf13/pflag"
)

type server struct {
	client  *meetupcom.Client
	urlName string
	cache   *cache.Cache
}

type attendee struct {
	ID        int64  `json:"id"`
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
		if item.Respone == "yes" {
			out.Yes = append(out.Yes, m)
		} else if item.Respone == "no" {
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
		json.NewEncoder(w).Encode(convertRSVPs(cached.(meetupcom.RSVPsResponse)))
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
	json.NewEncoder(w).Encode(convertRSVPs(*rsvps))
}

func main() {
	ctx := context.Background()
	var addr string
	var apiToken string
	var apiAuthCode string
	var apiRequestURI string
	var urlName string
	var allowedOrigins []string
	apiClientID, _ := os.LookupEnv("MEETUP_CLIENT_ID")
	apiClientSecret, _ := os.LookupEnv("MEETUP_CLIENT_SECRET")

	flag.StringVar(&addr, "addr", "127.0.0.1:8080", "Address to listen on")
	flag.StringVar(&apiToken, "api-token", "", "Meetup.com API refresh token")
	flag.StringVar(&apiAuthCode, "api-auth-code", "", "Auth code")
	flag.StringVar(&apiRequestURI, "api-request-uri", "", "API request URI for your application")
	flag.StringVar(&urlName, "url-name", "Graz-Open-Source-Meetup", "URL name of the meetup group on meetup.com")
	flag.StringArrayVar(&allowedOrigins, "allowed-origins", []string{"http://localhost:1313", "https://gograz.org"}, "Allowed origin hosts")
	flag.Parse()

	o := &oauth.OAuth2{ClientID: apiClientID, ClientSecret: apiClientSecret, RequestURI: apiRequestURI}

	if apiToken == "" {
		// If no API token is provided, then the client ID and client secret have to be present
		// in order for the user to generate a new access token.
		if apiAuthCode == "" {
			fmt.Println(o.GenerateAuthURL(ctx))
			os.Exit(0)
		} else {
			token, err := o.GetAccessToken(ctx, apiAuthCode)
			if err != nil {
				log.Fatalf("Failed to request access token: %s", err.Error())
			}
			fmt.Printf("Access token: %s\n", token)
			os.Exit(0)
		}
		log.Fatal("No --api-token provided")
	}

	o.RefreshToken = apiToken
	o.StartReload(ctx)

	ch := cache.New(5*time.Minute, 10*time.Minute)

	mc := meetupcom.NewClient(meetupcom.ClientOptions{OAuthClient: o})

	s := server{
		client:  mc,
		urlName: urlName,
		cache:   ch,
	}

	router := chi.NewRouter()
	c := cors.New(cors.Options{
		AllowedOrigins:   allowedOrigins,
		AllowCredentials: true,
	})
	router.Get("/{eventID}/rsvps", s.handleGetRSVPs)
	log.Infof("Starting HTTPD on %s", addr)
	http.ListenAndServe(addr, c.Handler(router))
}
