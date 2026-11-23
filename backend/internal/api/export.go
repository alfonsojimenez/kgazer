package api

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/alfonsojimenez/kgazer/backend/internal/store"
)

var unsafeFilenameChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// Response headers are sent lazily on the first row so that failures before
// any output (e.g. unknown topic) still produce a proper JSON error response.
type exportWriter struct {
	w        http.ResponseWriter
	rc       *http.ResponseController
	format   string
	filename string
	columns  []string
	csv      *csv.Writer
	rows     int
	started  bool
}

func newExportWriter(w http.ResponseWriter, r *http.Request, filename string, columns []string) (*exportWriter, error) {
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "json"
	}
	if format != "json" && format != "csv" {
		return nil, errors.New("format must be 'json' or 'csv'")
	}
	rc := http.NewResponseController(w)
	rc.SetWriteDeadline(time.Time{})
	return &exportWriter{
		w:        w,
		rc:       rc,
		format:   format,
		filename: unsafeFilenameChars.ReplaceAllString(filename, "_") + "." + format,
		columns:  columns,
	}, nil
}

func (e *exportWriter) start() error {
	e.started = true
	contentType := "application/json"
	if e.format == "csv" {
		contentType = "text/csv; charset=utf-8"
	}
	e.w.Header().Set("Content-Type", contentType)
	e.w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, e.filename))
	e.w.WriteHeader(http.StatusOK)
	if e.format == "csv" {
		e.csv = csv.NewWriter(e.w)
		return e.csv.Write(e.columns)
	}
	_, err := e.w.Write([]byte("["))
	return err
}

func (e *exportWriter) write(record any, csvRow []string) error {
	if !e.started {
		if err := e.start(); err != nil {
			return err
		}
	}
	e.rows++
	if e.format == "csv" {
		if err := e.csv.Write(csvRow); err != nil {
			return err
		}
	} else {
		if e.rows > 1 {
			if _, err := e.w.Write([]byte(",\n")); err != nil {
				return err
			}
		}
		encoded, err := json.Marshal(record)
		if err != nil {
			return err
		}
		if _, err := e.w.Write(encoded); err != nil {
			return err
		}
	}
	if e.rows%1000 == 0 {
		return e.flush()
	}
	return nil
}

func (e *exportWriter) flush() error {
	if e.csv != nil {
		e.csv.Flush()
		if err := e.csv.Error(); err != nil {
			return err
		}
	}
	return e.rc.Flush()
}

func (e *exportWriter) finish() error {
	if !e.started {
		if err := e.start(); err != nil {
			return err
		}
	}
	if e.format == "json" {
		if _, err := e.w.Write([]byte("]\n")); err != nil {
			return err
		}
	}
	return e.flush()
}

func formatTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func handleExportKeys(s *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		topic := chi.URLParam(r, "topic")
		cluster := r.URL.Query().Get("cluster")
		params, err := parseKeysParams(r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		ew, err := newExportWriter(w, r, topic+"-keys", []string{"key", "message_count", "partition", "offset", "last_updated", "deleted_at", "body"})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		err = s.ExportKeys(r.Context(), cluster, topic, params, func(k store.ExportedKey) error {
			return ew.write(k, []string{
				k.Key, strconv.Itoa(k.MessageCount), strconv.Itoa(int(k.Partition)), strconv.FormatInt(k.Offset, 10),
				formatTime(&k.LastUpdated), formatTime(k.DeletedAt), string(k.Body),
			})
		})
		if err == nil {
			err = ew.finish()
		}
		finishExport(w, ew, err, "exporting keys", topic)
	}
}

func handleExportKeyHistory(s *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		topic := chi.URLParam(r, "topic")
		cluster := r.URL.Query().Get("cluster")
		key := r.URL.Query().Get("key")
		if key == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "key query param required"})
			return
		}
		ew, err := newExportWriter(w, r, topic+"-"+key+"-history", []string{"key", "partition", "offset", "timestamp", "headers", "body"})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		err = s.ExportKeyHistory(r.Context(), cluster, topic, key, func(m store.Message) error {
			return ew.write(m, []string{
				m.Key, strconv.Itoa(int(m.Partition)), strconv.FormatInt(m.Offset, 10),
				formatTime(&m.Timestamp), string(m.Headers), string(m.Body),
			})
		})
		if err == nil {
			err = ew.finish()
		}
		finishExport(w, ew, err, "exporting key history", topic)
	}
}

func finishExport(w http.ResponseWriter, ew *exportWriter, err error, action, topic string) {
	if err == nil {
		return
	}
	if ew.started {
		slog.Error(action+" aborted mid-stream", "error", err, "topic", topic, "rows", ew.rows)
		return
	}
	if errors.Is(err, store.ErrTopicNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "topic not found"})
		return
	}
	slog.Error(action, "error", err, "topic", topic)
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
}
