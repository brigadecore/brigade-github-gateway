package webhooks

import (
	"encoding/json"
	"log"
	"net/http"

	// nolint: lll

	"github.com/google/go-github/v33/github"
)

var emptyResponse = []byte("{}")

// HandlerConfig encapsulates Handler configuration.
type HandlerConfig struct {
	// SharedSecret is the secret mutually agreed upon by this gateway and the
	// GitHub App that sends webhooks (events) to this gateway. This secret can be
	// used to validate the authenticity and integrity of payloads received by
	// this gateway.
	SharedSecret string
}

// Handler is an implementation of the http.Handler interface that can handle
// webhooks (events) from GitHub by delegating to a transport-agnostic Service
// interface.
type Handler struct {
	// Service is a transport-agnostic webhook (event) handler.
	Service Service
	// Config encapsulates configuration for this Handler.
	Config HandlerConfig
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	w.Header().Set("Content-Type", "application/json")

	payload, err := github.ValidatePayload(r, []byte(h.Config.SharedSecret))
	if err != nil {
		w.WriteHeader(http.StatusForbidden)
		w.Write(emptyResponse) // nolint: errcheck
		return
	}

	events, err := h.Service.Handle(r.Context(), github.WebHookType(r), payload)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		w.WriteHeader(http.StatusOK)
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

	w.Write(responseJSON) // nolint: errcheck
}
