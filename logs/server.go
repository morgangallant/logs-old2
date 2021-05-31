// `logs` is the server which both ingests logs and serves them on the web.
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	logger "log"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func must(key string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	panic("missing environment variable " + key)
}

func fallback(key, fv string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fv
}

// Initialized below.
var (
	databaseUrl      string
	lport            string
	telegramUsername string
	telegramSecret   string
	ownerName        string
)

func init() {
	_ = godotenv.Load()
	databaseUrl = must("DATABASE_URL")
	lport = fallback("PORT", "8080")
	telegramUsername = must("TELEGRAM_USERNAME")
	telegramSecret = must("TELEGRAM_SECRET")
	ownerName = fallback("OWNER_NAME", "John Doe")
}

func main() {
	if err := run(); err != nil {
		logger.Fatal(err)
	}
}

func doPostgresMigrations(conn *sql.DB) error {
	stmt := `CREATE TABLE IF NOT EXISTS logs (id SERIAL PRIMARY KEY, timestamp TIMESTAMPTZ, content TEXT);`
	_, err := conn.Exec(stmt)
	return err
}

func run() error {
	db, err := sql.Open("postgres", databaseUrl)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		return err
	}
	if err := doPostgresMigrations(db); err != nil {
		return err
	}
	http.HandleFunc("/", getHandler(db))
	http.HandleFunc("/_wh/telegram", telegramHandler(db))
	return http.ListenAndServe(":"+lport, nil)
}

type log struct {
	ts      time.Time
	content string
}

func fetchLogs(db *sql.DB) ([]log, error) {
	rows, err := db.Query("SELECT timestamp, content FROM logs ORDER BY timestamp desc")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	logs := []log{}
	for rows.Next() {
		var ts time.Time
		var content string
		if err := rows.Scan(&ts, &content); err != nil {
			return nil, err
		}
		logs = append(logs, log{ts: ts, content: content})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return logs, nil
}

func insertLog(db *sql.DB, l log) error {
	stmt := "INSERT INTO logs (timestamp, content) VALUES ($1, $2)"
	if _, err := db.Exec(stmt, l.ts, l.content); err != nil {
		return err
	}
	return nil
}

const timezone = "America/Toronto"

const (
	dayFormat  = "2006-01-02"
	timeFormat = "15:04"
)

func getHandler(db *sql.DB) http.HandlerFunc {
	tz, err := time.LoadLocation(timezone)
	if err != nil {
		panic(err)
	}
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		logs, err := fetchLogs(db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Fprintln(w, `<html lang="en">`)
		fmt.Fprintln(w, "<head>")
		fmt.Fprintln(w, `<meta charset="UTF-8" />`)
		fmt.Fprintln(w, `<meta name="viewport" content="width=device-width, initial-scale=1.0" />`)
		fmt.Fprintf(w, "<title>%s's Logs</title>\n", ownerName)
		fmt.Fprintln(w, "</head>")
		fmt.Fprintln(w, "<body>")
		fmt.Fprintln(w, "<div style=\"max-width: 960px; margin: 0 auto;\">")
		fmt.Fprintf(w, "<p><strong>%s's Logs</strong></p>\n", ownerName)
		fmt.Fprintf(w, "<p>Current TZ: %s.</p>\n", timezone)
		fmt.Fprintln(w, "<ul>")
		var prevday int
		for _, l := range logs {
			ts := l.ts.In(tz)
			if day := ts.Day(); day != prevday {
				fmt.Fprintf(w, "<p>%s</p>\n", ts.Format(dayFormat))
				prevday = day
			}
			fmt.Fprintf(w, "<li>%s: %s</li>\n", ts.Format(timeFormat), l.content)
		}
		fmt.Fprintln(w, "</ul>")
		fmt.Fprintf(w, "<p style=\"text-align: center;\">Rendered %d logs in %d ms.</p>", len(logs), time.Since(start).Milliseconds())
		fmt.Fprintln(w, "</div>")
		fmt.Fprintln(w, "</body>")
		fmt.Fprintln(w, "</html>")
		w.Header().Set("Content-Type", "text/html")
	}
}

func telegramHandler(db *sql.DB) http.HandlerFunc {
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
		if whkeys, ok := r.URL.Query()["key"]; !ok || len(whkeys) == 0 || whkeys[0] != telegramSecret {
			http.Error(w, "invalid secret key", http.StatusUnauthorized)
			return
		}
		var wh webhook
		if err := json.NewDecoder(r.Body).Decode(&wh); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if wh.Message.From.Username != telegramUsername {
			// If this message is from an unknown sender, ignore it.
			return
		}
		l := log{ts: time.Now(), content: wh.Message.Text}
		if err := insertLog(db, l); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}
