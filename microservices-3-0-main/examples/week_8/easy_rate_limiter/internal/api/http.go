package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
)

// HandleCreateUfo создаёт наблюдение (HTTP POST /ufo)
func (a *api) HandleCreateUfo(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "невалидный JSON", http.StatusBadRequest)
		return
	}

	if req.Title == "" {
		http.Error(w, "заголовок обязателен", http.StatusBadRequest)
		return
	}

	a.mu.Lock()
	id := a.nextID
	a.nextID++
	a.sightings[id] = &ufo{ID: id, Title: req.Title, Content: req.Content}
	a.mu.Unlock()

	slog.Info("наблюдение создано", "id", id)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(map[string]int64{"id": id}); err != nil {
		slog.Error("ошибка записи ответа", "error", err)
	}
}

// HandleGetUfo возвращает наблюдение по ID (HTTP GET /ufo/{id})
func (a *api) HandleGetUfo(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "невалидный ID", http.StatusBadRequest)
		return
	}

	a.mu.RLock()
	n, exists := a.sightings[id]
	a.mu.RUnlock()

	if !exists {
		http.Error(w, "наблюдение не найдено", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err = json.NewEncoder(w).Encode(n); err != nil {
		slog.Error("ошибка записи ответа", "error", err)
	}
}

// HandlePing возвращает статус сервиса (HTTP GET /ping)
func (a *api) HandlePing(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
		slog.Error("ошибка записи ответа", "error", err)
	}
}
