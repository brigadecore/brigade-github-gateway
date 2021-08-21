package webhooks

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/google/go-github/v33/github"
)

// handler is an implementation of the http.Handler interface that can handle
// webhooks (events) from GitHub by delegating to a transport-agnostic Service
// interface.
type handler struct {
	service Service
}

// NewHandler returns an implementation of the http.Handler interface that can
// handle webhooks (events) from GitHub by delegating to a transport-agnostic
// Service interface.
func NewHandler(service Service) http.Handler {
	return &handler{
		service: service,
	}
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	w.Header().Set("Content-Type", "application/json")

	payload, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	events, err := h.service.Handle(r.Context(), github.WebHookType(r), payload)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	responseObj := struct {
		EventIDs []string `json:"eventIDs"`
	}{
		EventIDs: make([]string, len(events.Items)),
	}
	for i, event := range events.Items {
		responseObj.EventIDs[i] = event.ID
	}

	responseJSON, _ := json.Marshal(responseObj)

	w.WriteHeader(http.StatusOK)
	w.Write(responseJSON) // nolint: errcheck
}
