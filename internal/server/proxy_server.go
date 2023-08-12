package server

import (
	"database/sql"
	"encoding/json"
	"http_server/internal/schema"
	"io/ioutil"
	"net/http"
	"net/url"
	"sync"
)

type ProxyServer struct {
	mu sync.Mutex
	DB *sql.DB
}

func NewProxyServer(db *sql.DB) *ProxyServer {
	return &ProxyServer{
		DB: db,
	}
}

func (p *ProxyServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Error reading request body", http.StatusBadRequest)
			return
		}

		var requestData schema.RequestData
		err = json.Unmarshal(body, &requestData)
		if err != nil {
			http.Error(w, "Invalid JSON data", http.StatusBadRequest)
			return
		}

		targetURL, err := url.Parse(requestData.URL)
		if err != nil {
			http.Error(w, "Invalid target URL", http.StatusBadRequest)
			return
		}

		req, err := http.NewRequest(requestData.Method, targetURL.String(), nil)
		if err != nil {
			http.Error(w, "Error creating request", http.StatusInternalServerError)
			return
		}
		for key, value := range requestData.Headers {
			req.Header.Set(key, value)
		}

		client := http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			http.Error(w, "Error sending request to target", http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()

		responseHeaders := make(map[string]string)
		for key, values := range resp.Header {
			responseHeaders[key] = values[0]
		}

		p.mu.Lock()
		defer p.mu.Unlock()

		tx, err := p.DB.Begin()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		stmt, err := tx.Prepare("INSERT INTO requests (method, url, headers) VALUES ($1, $2, $3) RETURNING id")
		if err != nil {
			tx.Rollback()
			http.Error(w, "Error preparing request insert statement", http.StatusInternalServerError)
			return
		}
		defer stmt.Close()
		headers, err := json.Marshal(requestData.Headers)
		if err != nil {
			tx.Rollback()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var requestID int
		err = stmt.QueryRow(requestData.Method, requestData.URL, string(headers)).Scan(&requestID)
		if err != nil {
			tx.Rollback()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		respHeaders, err := json.Marshal(responseHeaders)
		if err != nil {
			tx.Rollback()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, err = tx.Exec("INSERT INTO responses (request_id, status, headers, length) VALUES ($1, $2, $3, $4)",
			requestID, resp.StatusCode, string(respHeaders), resp.ContentLength)
		if err != nil {
			tx.Rollback()
			http.Error(w, "Error inserting response data", http.StatusInternalServerError)
			return
		}

		err = tx.Commit()
		if err != nil {
			http.Error(w, "Error committing transaction", http.StatusInternalServerError)
			return
		}

		responseData := schema.ResponseData{
			Status:  resp.StatusCode,
			Headers: responseHeaders,
			Length:  resp.ContentLength,
		}

		responseJSON, err := json.Marshal(responseData)
		if err != nil {
			http.Error(w, "Error encoding response JSON", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(responseJSON)
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
