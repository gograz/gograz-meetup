package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

type OAuth2 struct {
	ClientID        string
	ClientSecret    string
	RequestURI      string
	AuthCode        string
	RefreshToken    string
	accessToken     string
	accessTokenLock sync.RWMutex
}

func (o *OAuth2) CurrentAccessToken() string {
	o.accessTokenLock.RLock()
	defer o.accessTokenLock.RUnlock()
	return o.accessToken
}

type accessTokenResponse struct {
	AccessToken string `json:"access_token"`
	RefreshURL  string `json:"refresh_token"`
}

func (o *OAuth2) StartReload(ctx context.Context) {
	o.refreshAccessToken(ctx)
	go func() {
		ticker := time.NewTicker(time.Minute * 5)
		for {
			select {
			case _ = <-ticker.C:
				o.refreshAccessToken(ctx)
			case _ = <-ctx.Done():
				fmt.Println("Stopping refresher...")
				return
			}
		}
	}()
}

func (o *OAuth2) GenerateAuthURL(ctx context.Context) string {
	vals := url.Values{}
	vals.Set("client_id", o.ClientID)
	vals.Set("response_type", "code")
	vals.Set("redirect_uri", o.RequestURI)
	return fmt.Sprintf("https://secure.meetup.com/oauth2/authorize?%s", vals.Encode())
}

func (o *OAuth2) GetAccessToken(ctx context.Context, authCode string) (string, error) {
	vals := url.Values{}
	vals.Set("client_id", o.ClientID)
	vals.Set("client_secret", o.ClientSecret)
	vals.Set("grant_type", "authorization_code")
	vals.Set("redirect_uri", o.RequestURI)
	vals.Set("code", authCode)
	u := "https://secure.meetup.com/oauth2/access"

	resp, err := http.PostForm(u, vals)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected response code %d", resp.StatusCode)
	}
	defer resp.Body.Close()
	var a accessTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&a); err != nil {
		return "", err
	}
	return a.AccessToken, nil
}

func (o *OAuth2) refreshAccessToken(ctx context.Context) error {
	logrus.Info("Refreshing API access token")

	vals := url.Values{}
	vals.Set("client_id", o.ClientID)
	vals.Set("client_secret", o.ClientSecret)
	vals.Set("grant_type", "authorization_code")
	vals.Set("redirect_uri", o.RequestURI)
	vals.Set("refresh_token", o.RefreshToken)
	u := "https://secure.meetup.com/oauth2/access"

	resp, err := http.PostForm(u, vals)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected response code %d", resp.StatusCode)
	}
	defer resp.Body.Close()
	var a accessTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&a); err != nil {
		return err
	}
	o.accessTokenLock.Lock()
	defer o.accessTokenLock.Unlock()
	o.accessToken = a.AccessToken
	return nil
}
