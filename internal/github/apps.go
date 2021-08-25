package github

// App encapsulates the details of a GitHub App that sends webbooks to this
// gateway.
type App struct {
	// AppID specifies the ID of the GitHub App.
	AppID int64 `json:"appID"`
	// SharedSecret is the secret mutually agreed upon by this gateway and the
	// GitHub App. This secret can be used to validate the authenticity and
	// integrity of payloads received by this gateway.
	SharedSecret string `json:"sharedSecret"`
	// APIKey is the private API key for the GitHub App.
	APIKey string `json:"apiKey"`
}
