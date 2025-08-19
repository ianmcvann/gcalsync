package main

import (
	"fmt"
	"log"
)

func listCalendars() {
	db, err := openDB(".gcalsync.db")
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}
	defer db.Close()

	fmt.Println("📋 Here's the list of calendars you are syncing:")

	rows, err := db.Query("SELECT account_name, calendar_id, count(1) as num_events FROM blocker_events GROUP BY 1,2;")
	if err != nil {
		log.Fatalf("❌ Error retrieving blocker events from database: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var accountName, calendarID string
		var numEvents int
		if err := rows.Scan(&accountName, &calendarID, &numEvents); err != nil {
			log.Fatalf("❌ Unable to read calendar record or no calendars defined: %v", err)
		}
		fmt.Printf("  👤 %s (📅 %s) - %d\n", accountName, calendarID, numEvents)
	}

	// Display blocking relationships
	fmt.Println("\n🚫 Calendar blocking relationships (anonymous 'Busy' events):")
	blockRows, err := db.Query("SELECT source_calendar_id, target_calendar_id FROM calendar_blocks ORDER BY source_calendar_id, target_calendar_id")
	if err != nil {
		log.Printf("❌ Error retrieving calendar blocks: %v", err)
		return
	}
	defer blockRows.Close()

	hasBlocks := false
	for blockRows.Next() {
		hasBlocks = true
		var sourceCalendarID, targetCalendarID string
		if err := blockRows.Scan(&sourceCalendarID, &targetCalendarID); err != nil {
			log.Printf("❌ Error scanning block row: %v", err)
			continue
		}
		fmt.Printf("  🚫 %s → %s (anonymous 'Busy' events)\n", sourceCalendarID, targetCalendarID)
	}

	if !hasBlocks {
		fmt.Println("  ✅ No calendar blocks configured")
	}
}
