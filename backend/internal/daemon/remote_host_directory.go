package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// publishRemoteHostAddress posts a host's current quick-tunnel address to the
// cloud control plane's remote-host directory. The local-only build does not
// run the periodic publisher that called this (it was removed with the
// Connect Mobile wiring in Run); the primitive is kept for when remote access
// returns.
func publishRemoteHostAddress(ctx context.Context, client *http.Client, controlPlaneURL, hostID, address, hash string) error {
	base, err := url.Parse(controlPlaneURL)
	if err != nil {
		return fmt.Errorf("invalid control-plane URL")
	}
	localHTTP := base.Scheme == "http" && (base.Hostname() == "localhost" || base.Hostname() == "127.0.0.1")
	if base.Scheme != "https" && !localHTTP {
		return fmt.Errorf("invalid control-plane URL")
	}
	body, _ := json.Marshal(map[string]string{"url": address})
	target := strings.TrimRight(controlPlaneURL, "/") + "/api/cloud/v1/remote-hosts/" + url.PathEscape(hostID) + "/address"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+hash)
	req.Header.Set("Content-Type", "application/json")
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusNoContent {
		return fmt.Errorf("control plane returned %d", response.StatusCode)
	}
	return nil
}
