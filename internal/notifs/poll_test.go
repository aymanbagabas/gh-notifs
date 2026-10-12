package notifs

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestNotificationPollDeadline(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusNotModified} {
		for _, tc := range []struct {
			header string
			delay  time.Duration
		}{
			{"", time.Minute},
			{"1", time.Minute},
			{"120", 2 * time.Minute},
		} {
			t.Run(fmt.Sprintf("%d/%s", status, tc.header), func(t *testing.T) {
				c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("X-Poll-Interval", tc.header)
					w.WriteHeader(status)
					if status == http.StatusOK {
						fmt.Fprint(w, "[]")
					}
				})
				endpoint := Endpoint(Query{}, 1)
				c.Cache.Put(endpoint, Page{ETag: `"poll"`})
				before := time.Now()
				if _, err := c.page(context.Background(), endpoint, nil); err != nil {
					t.Fatal(err)
				}
				deadline := c.PollAfter()
				if deadline.Before(before.Add(tc.delay)) || deadline.After(time.Now().Add(tc.delay)) {
					t.Fatalf("poll deadline %v does not respect %v", deadline, tc.delay)
				}
				c.mu.Lock()
				c.blockedUntil = deadline.Add(time.Minute)
				c.mu.Unlock()
				if got := c.PollAfter(); got != deadline.Add(time.Minute) {
					t.Fatalf("poll ignored rate-limit deadline: %v", got)
				}
			})
		}
	}
}

func TestNotificationPollKeepsLongestDelay(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Poll-Interval", "60")
		fmt.Fprint(w, "[]")
	})
	later := time.Now().Add(5 * time.Minute)
	c.pollAfter = later
	if _, err := c.page(context.Background(), Endpoint(Query{}, 1), nil); err != nil {
		t.Fatal(err)
	}
	if c.PollAfter() != later {
		t.Fatal("another page shortened the polling delay")
	}
}

func TestInvalidNotificationPollInterval(t *testing.T) {
	for _, value := range []string{"bad", "-1", "0", "9223372036854775807"} {
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Poll-Interval", value)
			fmt.Fprint(w, "[]")
		})
		if _, err := c.page(context.Background(), Endpoint(Query{}, 1), nil); err == nil {
			t.Fatalf("accepted invalid polling interval %q", value)
		}
	}
}
