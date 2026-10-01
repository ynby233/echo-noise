package updates

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Wake only accelerates the existing claim protocol. The committed task stays
// authoritative, including when the scheduler cannot acknowledge this request.
func WakeExecutor() error {
	address := os.Getenv("UPDATE_EXECUTOR_WAKE_URL")
	if address == "" {
		return nil
	}
	u, err := url.Parse(address)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("executor_wake_configuration")
	}
	f, err := os.Open(os.Getenv("UPDATE_EXECUTOR_WAKE_TOKEN_FILE"))
	if err != nil {
		return errors.New("executor_wake_configuration")
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 4097))
	token := strings.TrimSpace(string(raw))
	if err != nil || token == "" || len(raw) > 4096 || strings.ContainsAny(token, "\r\n\t ") {
		return errors.New("executor_wake_configuration")
	}
	request, err := http.NewRequest(http.MethodPost, address, nil)
	if err != nil {
		return errors.New("executor_wake_configuration")
	}
	request.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return errors.New("executor_wake_transport")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return errors.New("executor_wake_rejected")
	}
	return nil
}
