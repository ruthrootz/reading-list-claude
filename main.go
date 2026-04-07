package main

import (
	"database/sql"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	_ "github.com/tursodatabase/libsql-client-go/libsql"
)

var db *sql.DB

var tabs = []string{"craft", "celebration", "contemplation", "constitution", "community"}

type URL struct {
	URL      string
	Title    string
	AddedAt  string
	Archived bool
	Tab      string
}

type PageData struct {
	Active       []URL
	Archived     []URL
	ShowArchived bool
	Tab          string
	Tabs         []string
}

var titleRe = regexp.MustCompile(`(?i)<title[^>]*>\s*(.*?)\s*</title>`)

func fetchTitle(pageURL string) string {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(pageURL)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return ""
	}
	matches := titleRe.FindSubmatch(body)
	if len(matches) >= 2 {
		return strings.TrimSpace(string(matches[1]))
	}
	return ""
}

func initDB() {
	dbURL := os.Getenv("TURSO_DATABASE_URL")
	token := os.Getenv("TURSO_AUTH_TOKEN")
	if dbURL == "" {
		log.Fatal("TURSO_DATABASE_URL is not set")
	}
	if token != "" {
		dbURL += "?authToken=" + token
	}

	var err error
	db, err = sql.Open("libsql", dbURL)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS urls (
		url      TEXT PRIMARY KEY,
		title    TEXT NOT NULL,
		added_at TEXT NOT NULL,
		archived INTEGER NOT NULL DEFAULT 0,
		tab      TEXT NOT NULL DEFAULT 'craft'
	)`)
	if err != nil {
		log.Fatalf("failed to create table: %v", err)
	}

	// Add tab column to existing tables that predate this feature.
	db.Exec(`ALTER TABLE urls ADD COLUMN tab TEXT NOT NULL DEFAULT 'craft'`)
}

func activeTab(r *http.Request) string {
	t := r.URL.Query().Get("tab")
	for _, tab := range tabs {
		if t == tab {
			return t
		}
	}
	return tabs[0]
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	tab := activeTab(r)
	rows, err := db.QueryContext(r.Context(), `SELECT url, title, added_at, archived FROM urls WHERE tab = ? ORDER BY added_at DESC`, tab)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	data := PageData{
		Tab:          tab,
		Tabs:         tabs,
		ShowArchived: r.URL.Query().Get("show") == "archived",
	}
	for rows.Next() {
		var u URL
		var archived int
		if err := rows.Scan(&u.URL, &u.Title, &u.AddedAt, &archived); err != nil {
			continue
		}
		if len(u.AddedAt) > 10 {
			u.AddedAt = u.AddedAt[:10]
		}
		u.Archived = archived == 1
		if u.Archived {
			data.Archived = append(data.Archived, u)
		} else {
			data.Active = append(data.Active, u)
		}
	}
	tmpl.Execute(w, data)
}

func handleAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	rawURL := strings.TrimSpace(r.FormValue("url"))
	if rawURL == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		rawURL = "https://" + rawURL
	}

	title := fetchTitle(rawURL)
	if title == "" {
		title = rawURL
	}

	tab := r.FormValue("tab")
	valid := false
	for _, t := range tabs {
		if tab == t {
			valid = true
			break
		}
	}
	if !valid {
		tab = tabs[0]
	}

	_, err := db.Exec(`INSERT OR IGNORE INTO urls (url, title, added_at, archived, tab) VALUES (?, ?, ?, 0, ?)`,
		rawURL, title, time.Now().UTC().Format(time.RFC3339), tab)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/?tab="+tab, http.StatusSeeOther)
}

func handleArchive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	target := r.FormValue("url")
	tab := r.FormValue("tab")

	_, err := db.Exec(`UPDATE urls SET archived = CASE WHEN archived = 1 THEN 0 ELSE 1 END WHERE url = ?`, target)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/?tab="+tab+"&show=archived", http.StatusSeeOther)
}

func handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	target := r.FormValue("url")
	tab := r.FormValue("tab")

	_, err := db.Exec(`DELETE FROM urls WHERE url = ?`, target)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/?tab="+tab, http.StatusSeeOther)
}

var tmpl = template.Must(template.New("index").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>reading list</title>
    <link rel="icon" type="image/svg+xml" href="data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 384 512'><path fill='%23c45e1a' d='M0 48V487.7C0 501.1 10.9 512 24.3 512c5 0 9.9-1.5 14-4.4L192 400 345.7 507.6c4.1 2.9 9 4.4 14 4.4c13.4 0 24.3-10.9 24.3-24.3V48c0-26.5-21.5-48-48-48H48C21.5 0 0 21.5 0 48z'/></svg>">
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Bodoni+Moda:ital,wght@0,400;0,500;0,600;1,400&display=swap">
    <link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/font-awesome/6.5.1/css/all.min.css">
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: "Bodoni Moda", "Didot", Georgia, serif;
            max-width: 700px;
            margin: 40px auto;
            padding: 0 20px;
            background: #1a1a1a;
            color: #e5e5e5;
        }
        h1 {
            font-size: 2.8rem;
            margin-bottom: 40px;
            text-align: center;
            color: #f0f0f0;
        }
        .add-form {
            display: flex;
            gap: 8px;
            margin-bottom: 48px;
        }
        .add-form input[type="text"] {
            flex: 1;
            padding: 10px 18px;
            font-size: 1rem;
            border: 1px solid #3a3a3a;
            border-radius: 999px;
            outline: none;
            background: #2a2a2a;
            color: #e5e5e5;
        }
        .add-form input[type="text"]:focus {
            border-color: #c45e1a;
            box-shadow: 0 0 0 3px rgba(196,94,26,0.2);
        }
        .add-form button {
            padding: 10px 16px;
            font-size: 1.3rem;
            background: #c45e1a;
            color: #fff;
            border: none;
            border-radius: 999px;
            cursor: pointer;
            display: flex;
            align-items: center;
            justify-content: center;
            line-height: 1;
        }
        .add-form button:hover { background: #a84e15; }
        .url-list { list-style: none; }
        .url-item {
            display: flex;
            align-items: center;
            justify-content: space-between;
            padding: 12px 20px;
            background: #242424;
            border: 1px solid #3a3a3a;
            border-radius: 999px;
            margin-bottom: 8px;
        }
        .url-item a {
            color: #e08a4a;
            text-decoration: none;
            font-weight: 600;
            white-space: nowrap;
            overflow: hidden;
            text-overflow: ellipsis;
            max-width: 460px;
        }
        @media (max-width: 600px) {
            .url-item a {
                max-width: 200px;
            }
        }
        .url-item a:hover { text-decoration: underline; color: #f0a060; }
        .url-meta {
            display: flex;
            align-items: center;
            gap: 12px;
            flex-shrink: 0;
            margin-left: 16px;
        }
        .url-date {
            font-size: 0.8rem;
            color: #777;
            white-space: nowrap;
        }
        .action-btn {
            background: none;
            border: none;
            color: #555;
            cursor: pointer;
            font-size: 0.85rem;
            padding: 4px 8px;
            border-radius: 4px;
        }
        .archive-btn:hover { color: #c45e1a; background: #2a2218; }
        .delete-btn:hover { color: #ef4444; background: #2a1a1a; }
        .toggle-row {
            display: flex;
            justify-content: flex-end;
            margin-bottom: 16px;
        }
        .toggle-btn {
            background: none;
            border: none;
            color: #777;
            cursor: pointer;
            font-size: 0.85rem;
            font-family: inherit;
            padding: 4px 0;
        }
        .toggle-btn:hover { color: #e08a4a; }
        .archived-items { display: none; }
        .archived-items.show { display: block; }
        .archived-items.init-show { display: block; }
        .archived-item a {
            text-decoration: line-through !important;
            color: #777 !important;
        }
        .archived-item a:hover { color: #999 !important; }
        .archived-item { opacity: 0.6; }
        .archived-item:hover { opacity: 1; }
        .empty {
            text-align: center;
            color: #555;
            padding: 40px 0;
        }
        .tabs {
            display: flex;
            gap: 6px;
            margin-bottom: 40px;
            flex-wrap: wrap;
            justify-content: center;
        }
        .tab-btn {
            background: #242424;
            border: 1px solid #333;
            color: #666;
            cursor: pointer;
            font-size: 1rem;
            font-family: inherit;
            letter-spacing: 0.06em;
            text-transform: lowercase;
            padding: 7px 18px;
            border-radius: 999px;
            text-decoration: none;
            transition: color 0.15s, border-color 0.15s, background 0.15s;
        }
        .tab-btn:hover {
            color: #e08a4a;
            border-color: #6b3a1f;
            background: #2a2218;
        }
        .tab-btn.active {
            background: #2a2218;
            color: #e08a4a;
            border-color: #c45e1a;
            box-shadow: 0 0 0 2px rgba(196,94,26,0.15);
        }
    </style>
</head>
<body>
    <h1>reading list</h1>

    <nav class="tabs">
        {{$currentTab := .Tab}}
        {{range .Tabs}}
        <a class="tab-btn{{if eq . $currentTab}} active{{end}}" href="/?tab={{.}}">{{.}}</a>
        {{end}}
    </nav>

    <form class="add-form" action="/add" method="POST">
        <input type="hidden" name="tab" value="{{.Tab}}">
        <input type="text" name="url" placeholder="enter a url...">
        <button type="submit" title="Save"><i class="fa-solid fa-bookmark"></i></button>
    </form>

    {{if .Archived}}
    <div class="toggle-row">
        <button class="toggle-btn" onclick="toggleArchived()">{{if .ShowArchived}}hide archived urls{{else}}show archived urls{{end}}</button>
    </div>
    {{end}}

    {{if .Active}}
    <ul class="url-list">
        {{range .Active}}
        <li class="url-item">
            <a href="{{.URL}}" target="_blank" rel="noopener">{{.Title}}</a>
            <div class="url-meta">
                <span class="url-date">{{.AddedAt}}</span>
                <form action="/archive" method="POST" style="display:inline">
                    <input type="hidden" name="url" value="{{.URL}}">
                    <input type="hidden" name="tab" value="{{$.Tab}}">
                    <button type="submit" class="action-btn archive-btn" title="Archive"><i class="fa-solid fa-box-archive"></i></button>
                </form>
                <form action="/delete" method="POST" style="display:inline">
                    <input type="hidden" name="url" value="{{.URL}}">
                    <input type="hidden" name="tab" value="{{$.Tab}}">
                    <button type="submit" class="action-btn delete-btn" title="Delete"><i class="fa-solid fa-trash"></i></button>
                </form>
            </div>
        </li>
        {{end}}
    </ul>
    {{else}}
    <p class="empty">No URLs saved yet. Add one above.</p>
    {{end}}

    {{if .Archived}}
    <div class="archived-items{{if .ShowArchived}} init-show{{end}}" id="archived-items">
        <ul class="url-list">
            {{range .Archived}}
            <li class="url-item archived-item">
                <a href="{{.URL}}" target="_blank" rel="noopener">{{.Title}}</a>
                <div class="url-meta">
                    <span class="url-date">{{.AddedAt}}</span>
                    <form action="/archive" method="POST" style="display:inline">
                        <input type="hidden" name="url" value="{{.URL}}">
                        <input type="hidden" name="tab" value="{{$.Tab}}">
                        <button type="submit" class="action-btn archive-btn" title="Unarchive"><i class="fa-solid fa-rotate-left"></i></button>
                    </form>
                    <form action="/delete" method="POST" style="display:inline">
                        <input type="hidden" name="url" value="{{.URL}}">
                        <input type="hidden" name="tab" value="{{$.Tab}}">
                        <button type="submit" class="action-btn delete-btn" title="Delete"><i class="fa-solid fa-trash"></i></button>
                    </form>
                </div>
            </li>
            {{end}}
        </ul>
    </div>
    {{end}}

    <script>
    function toggleArchived() {
        var el = document.getElementById('archived-items');
        var btn = document.querySelector('.toggle-btn');
        el.classList.toggle('show');
        btn.textContent = el.classList.contains('show') ? 'hide archived urls' : 'show archived urls';
    }
    </script>
</body>
</html>`))

func main() {
	initDB()

	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/add", handleAdd)
	http.HandleFunc("/archive", handleArchive)
	http.HandleFunc("/delete", handleDelete)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Listening on http://localhost:%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
