package meetupcom

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type Client struct {
	opts ClientOptions
}

type ClientOptions struct {
}

func NewClient(opts ClientOptions) *Client {
	c := Client{opts: opts}
	return &c
}

func (c *Client) executeGraphQLQuery(ctx context.Context, query string, variables map[string]string, output interface{}) error {
	u := "https://api.meetup.com/gql"
	body := make(map[string]interface{})
	body["query"] = query
	body["variables"] = variables
	requestBody := bytes.Buffer{}

	if err := json.NewEncoder(&requestBody).Encode(body); err != nil {
		return err
	}
	client := http.Client{}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, &requestBody)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("query returned unexpected status code %d", resp.StatusCode)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	return json.NewDecoder(resp.Body).Decode(output)
}
