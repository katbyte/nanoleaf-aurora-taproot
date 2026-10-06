package aurora

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// NewToken asks the controller for a token. The controller only hands one out
// for about 30 seconds after its power button has been held for 5 to 7
// seconds, until the light flashes; outside that window this returns
// ErrNotPairing, so a caller can ask again until the button has been held.
//
// A token never expires and gives full control of the controller. Treat it
// as a password.
func (c *Client) NewToken(ctx context.Context) (string, error) {
	const path = apiRoot + "/new"

	raw, err := c.send(ctx, http.MethodPost, path, path, "/new", nil, true)
	if err != nil {
		if se, ok := errors.AsType[*StatusError](err); ok && se.Status == http.StatusForbidden {
			return "", ErrNotPairing
		}
		return "", err
	}
	if raw == nil {
		return "", nil // a dry run
	}

	var out struct {
		Token string `json:"auth_token"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("POST /new: reading the answer: %w", err)
	}
	if out.Token == "" {
		return "", errors.New("POST /new: the controller answered without a token")
	}

	return out.Token, nil
}

// DeleteToken makes the controller forget the client's token. Nothing else
// the client does will work afterwards.
func (c *Client) DeleteToken(ctx context.Context) error {
	return c.del(ctx, "")
}
