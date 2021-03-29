package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"crawshaw.io/sqlite"
	"crawshaw.io/sqlite/sqlitex"

	_ "github.com/lib/pq"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func migrate(conn *sqlite.Conn) (err error) {
	defer sqlitex.Save(conn)(&err)
	err = sqlitex.Exec(conn, `
		CREATE TABLE IF NOT EXISTS logs (
			ts DATETIME NOT NULL,
			content TEXT NOT NULL
		);`, nil)
	return
}

func insertLog(conn *sqlite.Conn, ts time.Time, content string) (err error) {
	defer sqlitex.Save(conn)(&err)
	err = sqlitex.Exec(conn, `INSERT INTO logs (
		ts, content
	) VALUES (
		?, ?
	);`, nil, ts.Format(time.RFC3339), content)
	return
}

const oldUserID = "did:ethr:0x3CBD06ce22Df5749753002798030c033B98B574a"

// Only a one-time use, used to populate SQLite DB.
func fetchFromPostgres(conn *sqlite.Conn) (err error) {
	defer sqlitex.Save(conn)(&err)
	var db *sql.DB
	db, err = sql.Open("postgres", "host=localhost sslmode=disable")
	if err != nil {
		return
	}
	defer db.Close()
	var rows *sql.Rows
	rows, err = db.Query(`SELECT timestamp, content FROM text_logs WHERE user_id = $1;`, oldUserID)
	if err != nil {
		return
	}
	for rows.Next() {
		var ts time.Time
		var content string
		if err = rows.Scan(&ts, &content); err != nil {
			return
		}
		if err = insertLog(conn, ts, content); err != nil {
			return
		}
	}
	if err = rows.Close(); err != nil {
		return
	}
	err = rows.Err()
	return
}

const dbpath = "/root/storage/mglogs.db"

const addr = ":11108"

var dbpool *sqlitex.Pool

func run() error {
	var err error
	dbpool, err = sqlitex.Open(dbpath, 0, 10)
	if err != nil {
		return err
	}
	conn := dbpool.Get(context.TODO())
	if conn == nil {
		return errors.New("nil connection")
	}
	if err := migrate(conn); err != nil {
		return err
	}
	dbpool.Put(conn)
	log.Printf("Starting server.")
	http.HandleFunc("/", getHandler())
	http.HandleFunc("/_wh/telegram", telegramHandler())
	return http.ListenAndServe(addr, nil)
}

const currentTimezone = "America/Vancouver"

func tz() *time.Location {
	loc, err := time.LoadLocation(currentTimezone)
	if err != nil {
		panic(err)
	}
	return loc
}

const timeFormat = "2006-01-02 15:04"

func getHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		conn := dbpool.Get(r.Context())
		if conn == nil {
			return
		}
		defer dbpool.Put(conn)
		fmt.Fprintln(w, "<div style=\"width: 960px; margin: 0 auto;\">")
		fmt.Fprintln(w, "<p><strong>Morgan's Logs</strong></p>")
		loc := tz()
		fmt.Fprintln(w, "<ul>")
		stmt := conn.Prep(`SELECT ts, content FROM logs ORDER BY datetime(ts) DESC;`)
		for {
			if hasNext, err := stmt.Step(); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			} else if !hasNext {
				break
			}
			ts, err := time.Parse(time.RFC3339, stmt.GetText("ts"))
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			fmt.Fprintf(w, "<li>%s: %s</li>\n", ts.In(loc).Format(timeFormat), stmt.GetText("content"))
		}
		fmt.Fprintln(w, "</ul>")
		fmt.Fprintf(w, "<p style=\"text-align: center;\">Rendered in %d us.</p>", time.Since(start).Microseconds())
		fmt.Fprintln(w, "</div>")
		w.Header().Set("Content-Type", "text/html")
	}
}

func telegramHandler() http.HandlerFunc {
	type chat struct {
		ID int `json:"id"`
	}
	type from struct {
		ID        int    `json:"id"`
		IsBot     bool   `json:"is_bot"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
		Username  string `json:"username"`
	}
	type message struct {
		Text string `json:"text"`
		Chat chat   `json:"chat"`
		From from   `json:"from"`
	}
	type webhook struct {
		Message message `json:"message"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		conn := dbpool.Get(r.Context())
		if conn == nil {
			return
		}
		defer dbpool.Put(conn)
		var wh webhook
		if err := json.NewDecoder(r.Body).Decode(&wh); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if wh.Message.From.Username != "MorganGallant" {
			// Ignore.
			return
		}
		if err := insertLog(conn, time.Now(), wh.Message.Text); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}
