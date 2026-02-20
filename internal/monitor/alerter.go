package monitor

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

// AlertPayload is the JSON body sent to the webhook URL.
type AlertPayload struct {
	MonitorName string `json:"monitor_name"`
	URL         string `json:"url"`
	Status      string `json:"status"`
	Timestamp   string `json:"timestamp"`
}

// Alerter sends webhook notifications when a monitor transitions down.
type Alerter struct {
	webhookURL string
	client     *http.Client
}

// NewAlerter returns an Alerter that posts to webhookURL.
// If webhookURL is empty the alerter is a no-op.
func NewAlerter(webhookURL string) *Alerter {
	return &Alerter{
		webhookURL: webhookURL,
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

// Notify fires the webhook for a monitor that has just transitioned to down.
// It retries once after 5 s on failure.
func (a *Alerter) Notify(m *Monitor) {
	if a.webhookURL == "" {
		return
	}

	payload := AlertPayload{
		MonitorName: m.Name,
		URL:         m.URL,
		Status:      "down",
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}

	log.Printf("alert: monitor %q (%s) is DOWN — sending webhook to %s", m.Name, m.URL, a.webhookURL)

	if err := a.post(payload); err != nil {
		log.Printf("alert: webhook failed (%v) — retrying in 5s", err)
		time.Sleep(5 * time.Second)
		if err := a.post(payload); err != nil {
			log.Printf("alert: webhook retry failed: %v", err)
		} else {
			log.Printf("alert: webhook retry succeeded for monitor %q", m.Name)
		}
	}
}

func (a *Alerter) post(payload AlertPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	resp, err := a.client.Post(a.webhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
