package main

import (
	"fmt"
	"log"
	"os"
)

func blockCalendar() {
	if len(os.Args) < 4 {
		fmt.Println("Usage: gcalsync block <source_calendar_id> <target_calendar_id>")
		fmt.Println("This will create anonymous 'Busy' events in target_calendar (without event details from source_calendar)")
		os.Exit(1)
	}

	sourceCalendarID := os.Args[2]
	targetCalendarID := os.Args[3]

	db, err := openDB(".gcalsync.db")
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}
	defer db.Close()

	// Check if calendars exist in the system
	var sourceExists, targetExists bool
	err = db.QueryRow("SELECT 1 FROM calendars WHERE calendar_id = ?", sourceCalendarID).Scan(&sourceExists)
	if err != nil {
		fmt.Printf("❌ Source calendar %s not found in gcalsync. Add it first with 'gcalsync add'.\n", sourceCalendarID)
		os.Exit(1)
	}

	err = db.QueryRow("SELECT 1 FROM calendars WHERE calendar_id = ?", targetCalendarID).Scan(&targetExists)
	if err != nil {
		fmt.Printf("❌ Target calendar %s not found in gcalsync. Add it first with 'gcalsync add'.\n", targetCalendarID)
		os.Exit(1)
	}

	// Check if blocking relationship already exists
	var exists int
	err = db.QueryRow("SELECT 1 FROM calendar_blocks WHERE source_calendar_id = ? AND target_calendar_id = ?", 
		sourceCalendarID, targetCalendarID).Scan(&exists)
	if err == nil {
		fmt.Printf("⚠️ Calendar %s is already blocked from %s\n", sourceCalendarID, targetCalendarID)
		return
	}

	// Insert blocking relationship
	_, err = db.Exec("INSERT INTO calendar_blocks (source_calendar_id, target_calendar_id) VALUES (?, ?)", 
		sourceCalendarID, targetCalendarID)
	if err != nil {
		log.Fatalf("Error inserting calendar block: %v", err)
	}

	fmt.Printf("🚫 Blocked: Events from %s will now create anonymous 'Busy' events in %s (no event details)\n", 
		sourceCalendarID, targetCalendarID)
	fmt.Println("💡 Run 'gcalsync sync' to apply changes")
}

func unblockCalendar() {
	if len(os.Args) < 4 {
		fmt.Println("Usage: gcalsync unblock <source_calendar_id> <target_calendar_id>")
		fmt.Println("This will show full event details from source_calendar in target_calendar again (instead of just 'Busy')")
		os.Exit(1)
	}

	sourceCalendarID := os.Args[2]
	targetCalendarID := os.Args[3]

	db, err := openDB(".gcalsync.db")
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}
	defer db.Close()

	// Check if blocking relationship exists
	var exists int
	err = db.QueryRow("SELECT 1 FROM calendar_blocks WHERE source_calendar_id = ? AND target_calendar_id = ?", 
		sourceCalendarID, targetCalendarID).Scan(&exists)
	if err != nil {
		fmt.Printf("⚠️ No blocking relationship found between %s and %s\n", sourceCalendarID, targetCalendarID)
		return
	}

	// Remove blocking relationship
	result, err := db.Exec("DELETE FROM calendar_blocks WHERE source_calendar_id = ? AND target_calendar_id = ?", 
		sourceCalendarID, targetCalendarID)
	if err != nil {
		log.Fatalf("Error removing calendar block: %v", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected > 0 {
		fmt.Printf("✅ Unblocked: Events from %s will now show full details in %s (no longer anonymous 'Busy')\n", 
			sourceCalendarID, targetCalendarID)
		fmt.Println("💡 Run 'gcalsync sync' to apply changes")
	} else {
		fmt.Printf("⚠️ No blocking relationship found between %s and %s\n", sourceCalendarID, targetCalendarID)
	}
}

func listBlocks() {
	db, err := openDB(".gcalsync.db")
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}
	defer db.Close()

	fmt.Println("🚫 Current calendar blocking relationships (anonymous 'Busy' events):")

	rows, err := db.Query("SELECT source_calendar_id, target_calendar_id FROM calendar_blocks ORDER BY source_calendar_id, target_calendar_id")
	if err != nil {
		log.Fatalf("Error retrieving calendar blocks: %v", err)
	}
	defer rows.Close()

	hasBlocks := false
	for rows.Next() {
		hasBlocks = true
		var sourceCalendarID, targetCalendarID string
		if err := rows.Scan(&sourceCalendarID, &targetCalendarID); err != nil {
			log.Fatalf("Error scanning block row: %v", err)
		}
		fmt.Printf("  🚫 %s → %s (anonymous 'Busy' events)\n", sourceCalendarID, targetCalendarID)
	}

	if !hasBlocks {
		fmt.Println("  ✅ No calendar blocks configured")
	}
}