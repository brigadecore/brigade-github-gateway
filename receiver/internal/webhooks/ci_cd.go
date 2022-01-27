package webhooks

import (
	"strings"

	"github.com/brigadecore/brigade/sdk/v2/core"
	"github.com/google/go-github/v33/github"
)

func (s *service) getCICDEvent(event core.Event) *core.Event {
	ciCDEvent := &event
	switch event.Type {
	// Where check_suite:requested and check_suite:rerequested are concerned, it's
	// rare that any subscriber needs to differentiate between the two, so to
	// simplify matters for most subscribers, we collapse those two cases into a
	// ci:pipeline_requested event and we emit that in addition to the original
	// check_suite:requested or check_suite:rerequested event.
	case "check_suite:requested", "check_suite:rerequested":
		ciCDEvent.Type = "ci:pipeline_requested"
	// For consistency with the above, we emit a ci:job_requested event in
	// addition to the original check_run:rerequested event.
	case "check_run:rerequested":
		ciCDEvent.Type = "ci:job_requested"
		// We should always be able to parse this. If this were not parsable, we
		// would have encountered an error long before this function was called.
		// nolint: errcheck
		webhook, _ := github.ParseWebHook("check_run", []byte(event.Payload))
		// If we're already emitting a check_run:rerequested, the original webhook
		// MUST have been a *github.CheckRunEvent, so we can safely assume this
		// type assertion always succeeds.
		// nolint: forcetypeassert
		checkRunWebHook := webhook.(*github.CheckRunEvent)
		// Job names from this gateway are always of the form <project:job>
		qualifiedJobName := checkRunWebHook.GetCheckRun().GetName()
		jobNameTokens := strings.SplitN(qualifiedJobName, ":", 2)
		ciCDEvent.Labels["job"] = jobNameTokens[1]
	// For consistency with the above, we emit a cd:pipeline_requested event in
	// addition to the original release:published event.
	case "release:published":
		ciCDEvent.Type = "cd:pipeline_requested"
	default:
		return nil
	}
	return ciCDEvent
}
