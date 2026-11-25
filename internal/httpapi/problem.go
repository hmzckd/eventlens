package httpapi

import (
	"encoding/json"
	"net/http"
)

type problem struct {
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail"`
	Errors any    `json:"errors,omitempty"`
}

func writeProblem(w http.ResponseWriter, status int, title, detail string, errs any) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(problem{Title: title, Status: status, Detail: detail, Errors: errs})
}
