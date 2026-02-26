package main

import (
	"database/sql"
	"log"

	_ "modernc.org/sqlite"
)

// opens sqlite database and ensure schema
func InitDatabase(path string) *sql.DB {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		log.Panicln("error: could not open database:", err)
	}

	if err := db.Ping(); err != nil {
		log.Panicln("error: could not ping database:", err)
	}

	// create the table if it doesn't exist
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS slot_received_items (
			slot_id  TEXT NOT NULL,
			item_id  TEXT NOT NULL,
			PRIMARY KEY (slot_id, item_id)
		)
	`)
	if err != nil {
		log.Panicln("error: could not create table:", err)
	}

	log.Println("info: database initialized at", path)
	return db
}

// reads every row from the table and returns a map[slotID][]itemID
func LoadReceivedItems(db *sql.DB) map[string][]string {
	rows, err := db.Query("SELECT slot_id, item_id FROM slot_received_items")
	if err != nil {
		log.Panicln("error: could not query received items:", err)
	}
	defer rows.Close()

	result := make(map[string][]string)
	for rows.Next() {
		var slotID, itemID string
		if err := rows.Scan(&slotID, &itemID); err != nil {
			log.Panicln("error: could not scan row:", err)
		}
		result[slotID] = append(result[slotID], itemID)
	}

	if err := rows.Err(); err != nil {
		log.Panicln("error: row iteration error:", err)
	}

	count := 0
	for _, items := range result {
		count += len(items)
	}
	log.Printf("info: loaded %d received items for %d slots from database", count, len(result))

	return result
}

// inserts a single new item for a slot
// the PRIMARY KEY constraint prevents duplicates so we use INSERT OR IGNORE
func SaveReceivedItem(db *sql.DB, slotID, itemID string) {
	_, err := db.Exec(
		"INSERT OR IGNORE INTO slot_received_items (slot_id, item_id) VALUES (?, ?)",
		slotID, itemID,
	)
	if err != nil {
		log.Println("error: could not insert received item:", err)
	}
}
