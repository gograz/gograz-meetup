package meetupcom

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/gograz/gograz-meetup/pkg/oauth"
)

type Client struct {
	opts ClientOptions
}

type ClientOptions struct {
	OAuthClient *oauth.OAuth2
}

func NewClient(opts ClientOptions) *Client {
	c := Client{opts: opts}
	return &c
}

func (c *Client) executeGet(ctx context.Context, path string, query url.Values) (*http.Response, error) {
	u := url.URL{Scheme: "https", Host: "api.meetup.com", Path: path, RawQuery: query.Encode()}
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.opts.OAuthClient.CurrentAccessToken()))
	return http.DefaultClient.Do(req.WithContext(ctx))
}
