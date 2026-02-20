package monitor

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"
)

// failureThreshold is the number of consecutive failures before a monitor flips to "down".
const failureThreshold = 3

// Checker manages a pool of goroutines that periodically probe monitors.
type Checker struct {
	store  *Store
	mu     sync.Mutex
	cancel map[int64]context.CancelFunc
	wg     sync.WaitGroup
}

// NewChecker creates a Checker backed by store.
func NewChecker(store *Store) *Checker {
	return &Checker{
		store:  store,
		cancel: make(map[int64]context.CancelFunc),
	}
}

// Start loads all existing monitors from the DB and begins background probing.
// It also starts a 6-hour ticker to prune checks older than 7 days.
func (c *Checker) Start(ctx context.Context) error {
	monitors, err := c.store.List()
	if err != nil {
		return err
	}
	for _, m := range monitors {
		c.startWorker(ctx, m)
	}

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := c.store.PruneOldChecks(); err != nil {
					log.Printf("checker: prune old checks: %v", err)
				}
			}
		}
	}()

	return nil
}

// Add starts a background worker for a newly-created monitor.
func (c *Checker) Add(ctx context.Context, m *Monitor) {
	c.startWorker(ctx, m)
}

// Restart stops and re-starts the worker for a monitor (e.g. after an update).
func (c *Checker) Restart(ctx context.Context, m *Monitor) {
	c.stopWorker(m.ID)
	c.startWorker(ctx, m)
}

// Remove stops the background worker for a deleted monitor.
func (c *Checker) Remove(id int64) {
	c.stopWorker(id)
}

// Stop cancels all workers and waits for them to exit.
func (c *Checker) Stop() {
	c.mu.Lock()
	ids := make([]int64, 0, len(c.cancel))
	for id := range c.cancel {
		ids = append(ids, id)
	}
	c.mu.Unlock()
	for _, id := range ids {
		c.stopWorker(id)
	}
	c.wg.Wait()
}

func (c *Checker) startWorker(ctx context.Context, m *Monitor) {
	workerCtx, cancel := context.WithCancel(ctx)
	c.mu.Lock()
	c.cancel[m.ID] = cancel
	c.mu.Unlock()

	id := m.ID
	interval := time.Duration(m.IntervalSeconds) * time.Second
	timeout := time.Duration(m.TimeoutSeconds) * time.Second
	url := m.URL

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		// Small stagger on first check to avoid thundering herd at startup.
		select {
		case <-workerCtx.Done():
			return
		case <-time.After(time.Second):
		}

		// Probe immediately, then on each tick.
		c.probe(workerCtx, id, url, timeout)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-workerCtx.Done():
				return
			case <-ticker.C:
				c.probe(workerCtx, id, url, timeout)
			}
		}
	}()
}

func (c *Checker) stopWorker(id int64) {
	c.mu.Lock()
	cancel, ok := c.cancel[id]
	if ok {
		delete(c.cancel, id)
	}
	c.mu.Unlock()
	if ok {
		cancel()
	}
}

func (c *Checker) probe(ctx context.Context, monitorID int64, url string, timeout time.Duration) {
	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		log.Printf("monitor %d: build request for %q: %v", monitorID, url, err)
		return
	}
	req.Header.Set("User-Agent", "health-dashboard/1.0")

	start := time.Now()
	resp, httpErr := client.Do(req)
	ms := int(time.Since(start).Milliseconds())

	check := Check{
		MonitorID:      monitorID,
		ResponseTimeMs: &ms,
	}
	if httpErr == nil {
		resp.Body.Close()
		code := resp.StatusCode
		check.StatusCode = &code
		check.IsUp = code >= 200 && code < 400
	}
	// httpErr != nil → IsUp stays false, StatusCode stays nil.

	if err := c.store.RecordCheck(&check); err != nil {
		log.Printf("monitor %d: record check: %v", monitorID, err)
		return
	}

	c.updateState(monitorID, check.IsUp)
}

func (c *Checker) updateState(monitorID int64, isUp bool) {
	m, err := c.store.Get(monitorID)
	if err != nil || m == nil {
		return
	}

	var newState string
	var failures int

	if isUp {
		newState = "up"
		failures = 0
	} else {
		failures = m.ConsecutiveFailures + 1
		if failures >= failureThreshold {
			newState = "down"
		} else {
			// Not enough consecutive failures yet — hold current state.
			newState = m.State
		}
	}

	if err := c.store.UpdateState(monitorID, newState, failures); err != nil {
		log.Printf("monitor %d: update state: %v", monitorID, err)
	}
}
