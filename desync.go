package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

func desyncCalendars() {
	config, err := readConfig(".gcalsync.toml")
	if err != nil {
		log.Fatalf("Error reading config file: %v", err)
	}

	initOAuthConfig(config)

	ctx := context.Background()
	db, err := openDB(".gcalsync.db")
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}
	defer db.Close()

	fmt.Println("🚀 Starting calendar desynchronization...")

	rows, err := db.Query("SELECT DISTINCT event_id, calendar_id, account_name FROM blocker_events")
	if err != nil {
		log.Fatalf("❌ Error retrieving blocker events from database: %v", err)
	}
	defer rows.Close()

	// Group events by account to avoid duplicate client creation
	eventsByAccount := make(map[string][]struct {
		EventID    string
		CalendarID string
	})

	for rows.Next() {
		var eventID, calendarID, accountName string
		if err := rows.Scan(&eventID, &calendarID, &accountName); err != nil {
			log.Fatalf("❌ Error scanning blocker event row: %v", err)
		}

		eventsByAccount[accountName] = append(eventsByAccount[accountName], struct {
			EventID    string
			CalendarID string
		}{EventID: eventID, CalendarID: calendarID})
	}

	var eventIDCalendarIDPairs []struct {
		EventID    string
		CalendarID string
	}

	// Rate limiting: More conservative - 300 per minute = 5 per second = 200ms between requests
	rateLimiter := time.NewTicker(400 * time.Millisecond)
	defer rateLimiter.Stop()

	// Process events grouped by account to reuse clients
	for accountName, events := range eventsByAccount {
		fmt.Printf("🗑️ Processing %d events for account: %s\n", len(events), accountName)

		client := getClient(ctx, oauthConfig, db, accountName, config)
		calendarService, err := calendar.NewService(ctx, option.WithHTTPClient(client))
		if err != nil {
			log.Fatalf("❌ Error creating calendar client: %v", err)
		}

		for _, event := range events {
			// Rate limit the API calls
			<-rateLimiter.C

			// Retry logic with exponential backoff
			success := false
			for attempt := 0; attempt < 3; attempt++ {
				err = calendarService.Events.Delete(event.CalendarID, event.EventID).Do()

				if err == nil {
					fmt.Printf("  ✅ Blocker event deleted: %s\n", event.EventID)
					success = true
					break
				}

				if googleErr, ok := err.(*googleapi.Error); ok {
					switch googleErr.Code {
					case 404:
						fmt.Printf("  ⚠️ Blocker event not found in calendar: %s\n", event.EventID)
						success = true // Event doesn't exist, consider it "deleted"
					case 410:
						fmt.Printf("  ⚠️ Blocker event already deleted from calendar: %s\n", event.EventID)
						success = true // Event already gone, consider it "deleted"
					case 403, 429:
						if attempt < 2 {
							// More aggressive backoff: 2^(attempt+2) seconds
							backoffDelay := time.Duration(2<<(attempt+1)) * time.Second
							if googleErr.Code == 403 {
								fmt.Printf("  ⚠️ Rate limit exceeded, retrying in %v... (attempt %d/3)\n", backoffDelay, attempt+1)
							} else {
								fmt.Printf("  ⚠️ Too many requests, retrying in %v... (attempt %d/3)\n", backoffDelay, attempt+1)
							}
							time.Sleep(backoffDelay)
							continue
						}
						if googleErr.Code == 403 {
							log.Fatalf("❌ Rate limit exceeded after 3 attempts for event: %s", event.EventID)
						} else {
							log.Fatalf("❌ Too many requests after 3 attempts for event: %s", event.EventID)
						}
					default:
						log.Fatalf("❌ Error deleting blocker event: %v", err)
					}
				} else {
					log.Fatalf("❌ Error deleting blocker event: %v", err)
				}
				if success {
					break
				}
			}

			// If event was successfully deleted (or already gone), add to cleanup list
			if success {
				eventIDCalendarIDPairs = append(eventIDCalendarIDPairs, struct {
					EventID    string
					CalendarID string
				}{EventID: event.EventID, CalendarID: event.CalendarID})
			}
		}
	}

	// Delete blocker events from the database after the iteration
	fmt.Printf("📥 Cleaning up %d events from database...\n", len(eventIDCalendarIDPairs))
	for _, pair := range eventIDCalendarIDPairs {
		_, err := db.Exec("DELETE FROM blocker_events WHERE event_id = ? AND calendar_id = ?", pair.EventID, pair.CalendarID)
		if err != nil {
			log.Fatalf("❌ Error deleting blocker event from database: %v", err)
		} else {
			fmt.Printf("  📥 Blocker event deleted from database: %s\n", pair.EventID)
		}
	}

	fmt.Println("Calendars desynced successfully")
}

func getAccountNameByCalendarID(db *sql.DB, calendarID string) string {
	var accountName string
	err := db.QueryRow("SELECT account_name FROM calendars WHERE calendar_id = ?", calendarID).Scan(&accountName)
	if err != nil {
		log.Fatalf("Error retrieving account name for calendar ID %s: %v", calendarID, err)
	}
	return accountName
}
