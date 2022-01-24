# Event Reference

This section exists primarily for reference purposes and documents all GitHub
webhooks which can be handled by this gateway and their corresponding Brigade
events that may be emitted into the Brigade event bus.

The transformation of a webhook into an event is relatively straightforward and
subject to a few very simple rules:

1. In most cases, the event's `type` field will directly match the the webhook's
   type as determined by the value of the `X-GitHub-Event` HTTP header.

   For the relatively few webhooks whose JSON payload contains an `action`
   field, the corresponding event will have a `type` of the form
   `<webhook type>:<action>`. For instance, for a webhook of type `pull_request`
   with the value `opened` in the `action` field, the corresponding event would
   be of type `pull_request:opened`. This approach is used because this
   gateway's creators have observed that when a webhook is qualified by an
   `action` field, the processing a script author might wish to perform for one
   value of the `action` field typically varies significantly from the
   processing they might wish to perform for some _other_ value of the `action`
   field. By way of example, the logic required for handling a newly opened pull
   request might be quite different from the logic required for handling a pull
   request that's been closed. With this being the case, this gateway's creators
   considered it prudent to map each webhook type \+ action combination to its
   own distinct Brigade event type.

1. With every webhook handled by this gateway being indicative of activity
   involving some specific repository, the name of the affected repository is
   copied from the webhook's JSON payload and promoted to the `repo` qualifier
   on the corresponding event. This permits projects to subscribe to events
   relating only to specific repositories. Read more about qualifiers
   [here](https://docs.brigade.sh/topics/project-developers/events/#qualifiers).

1. For any webhook that is indicative of activity involving not only a specific
   repository, but also some specific ref (branch or tag) or commit (identified
   by SHA), this gateway copies those details from the webhook's JSON payload
   and:
   
    1. Promotes them to the corresponding event's `git.ref` and/or `git.commit`
       fields. By doing so, Brigade is enabled to locate specific code
       referenced by the webhook/event. The importance of this cannot be
       understated, as it is what permits Brigade to be used for implementing
       CI/CD pipelines.

    2. Promotes the ref to the `ref` label on the corresponding event. This
       permits projects to, for instance, subscribe only to events relating
       to a specific branch. Read more about labels
       [here](https://docs.brigade.sh/topics/project-developers/events/#labels).

1. If this gateway is able to infer that a webhook pertains only to a _specific_
   Brigade project, this information will be included in the corresponding
   event's `projectID` field and will effectively limit delivery of the event to
   the applicable Brigade project.

1. If this gateway is able to infer a human-friendly title for any webhook, the
   corresponding event will be augmented with values in its `shortTitle` and
   `longTitle` fields.

1. For _all_ webhooks, without exception, the entire JSON payload, without any
   modification, becomes the corresponding event's `payload`. The event
   `payload` field is a string field, however, so script authors wishing to
   access the payload will need to parse the payload themselves with a
   `JSON.parse()` call or similar.

1. For the relatively few webhooks that are of particular importance when
   utilizing Brigade to implement CI/CD pipelines, all previous rules apply, but
   _additional_ events may also be emitted into Brigade's event bus. See the
   [CI/CD](CI_CD.md) documentation for more details.

The following table summarizes all GitHub webhooks that can be handled by this
gateway and the corresponding event(s) that are emitted into Brigade's event bus.

> ⭐️&nbsp;&nbsp;symbols in the table call attention to the few events that are
> most relevant to the most common use case: CI/CD.

| Webhook | Scope | Possible Action Values | Event Type(s) Emitted |
|---------|-------|------------------------|-----------------------|
| [`check_run`](https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#check_run) | specific commit | <ul><li>`created`</li><li>`completed`</li><li>`rerequested`</li><li>`rerequested_action`</li></ul> | <ul><li>`check_run:created`</li><li>`check_run:completed`</li><li>`check_run:rerequested` + ⭐️&nbsp;&nbsp;`ci:job_requested`</li><li>`check_run:rerequested_action`</li></ul>
| [`check_suite`](https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#check_suite) | specific commit | <ul><li>`completed`</li><li>`requested`</li><li>`rerequested`</li></ul> | <ul><li>`check_suite:completed`</li><li>`check_suite:requested` + `ci:pipeline_requested`</li><li>`check_suite:rerequested` + ⭐️&nbsp;&nbsp;`ci:pipeline_requested`</li></ul>
| [`create`](https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#create) | specific branch or tag || <ul><li>`create`</li></ul>
| [`deleted`](https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#delete) | specific branch or tag || <ul><li>`deleted`</li></ul>
| [`fork`](https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#fork) | specific repository || <ul><li>`fork`</li></ul>
| [`gollum`](https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#gollum) | specific repository || <ul><li>`gollum`</li></ul>
| [`installation`](https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#installation) | multiple specific repositories; the gateway will split this into multiple repository-specific events | <ul><li>`created`</li><li>`deleted`</li><li>`suspend`</li><li>`unsuspend`</li><li>`new_permissions_accepted`</li></ul> | <ul><li>`installation:created`</li><li>`installation:deleted`</li><li>`installation:suspend`</li><li>`installation:unsuspend`</li><li>`installation:new_permissions_accepted`</li></ul>
| [`installation_repositories`](https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#installation_repositories) | multiple specific repositories; the gateway will split this into multiple repository-specific events | <ul><li>`added`</li><li>`removed`</li></ul> | <ul><li>`installation_repositories:added`</li><li>`installation_repositories:removed`</li></ul>
| [`issue_comment`](https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#issue_comment) | specific repository | <ul><li>`created`</li><li>`edited`</li><li>`deleted`</li></ul> | <ul><li>`issue_comment:created`</li><li>`issue_comment:edited`</li><li>`issue_comment:deleted`</li></ul>
| [`issues`](https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#issues) | specific repository | <ul><li>`opened`</li><li>`edited`</li><li>`deleted`</li><li>`pinned`</li><li>`unpinned`</li><li>`closed`</li><li>`reopened`</li><li>`assigned`</li><li>`labeled`</li><li>`unlabeled`</li><li>`locked`</li><li>`unlocked`</li><li>`transferred`</li><li>`milestoned`</li><li>`demilestoned`</li></ul> | <ul><li>`issues:opened`</li><li>`issues:edited`</li><li>`issues:deleted`</li><li>`issues:pinned`</li><li>`issues:unpinned`</li><li>`issues:closed`</li><li>`issues:reopened`</li><li>`issues:assigned`</li><li>`issues:labeled`</li><li>`issues:unlabeled`</li><li>`issues:locked`</li><li>`issues:unlocked`</li><li>`issues:transferred`</li><li>`issues:milestoned`</li><li>`issues:demilestoned`</li></ul>
| [`label`](https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#label) | specific repository | <ul><li>`created`</li><li>`edited`</li><li>`deleted`</li></ul> | <ul><li>`label:created`</li><li>`label:edited`</li><li>`label:deleted`</li></ul>
| [`member`](https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#member) | specific repository | <ul><li>`added`</li><li>`removed`</li><li>`edited`</li></ul> | <ul><li>`member:added`</li><li>`member:removed`</li><li>`member:edited`</li></ul>
| [`milestone`](https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#milestone) | specific repository | <ul><li>`created`</li><li>`closed`</li><li>`opened`</li><li>`edited`</li><li>`deleted`</li></ul> | <ul><li>`milestone:created`</li><li>`milestone:closed`</li><li>`milestone:opened`</li><li>`milestone:edited`</li><li>`milestone:deleted`</li></ul>
| [`page_build`](https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#page_build) | specific repository || <ul><li>`page_build`</li></ul>
| [`project_card`](https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#project_card) | specific repository | <ul><li>`created`</li><li>`edited`</li><li>`moved`</li><li>`converted`</li><li>`deleted`</li></ul> | <ul><li>`project_card:created`</li><li>`project_card:edited`</li><li>`project_card:moved`</li><li>`project_card:converted`</li><li>`project_card:deleted`</li></ul>
| [`project_column`](https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#project_column) | specific repository | <ul><li>`created`</li><li>`edited`</li><li>`moved`</li><li>`deleted`</li></ul> | <ul><li>`project_column:created`</li><li>`project_column:edited`</li><li>`project_column:moved`</li><li>`project_column:deleted`</li></ul>
| [`project`](https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#project) | specific repository | <ul><li>`created`</li><li>`edited`</li><li>`closed`</li><li>`reopened`</li><li>`deleted`</li></ul> | <ul><li>`project:created`</li><li>`project:edited`</li><li>`project:closed`</li><li>`project:reopened`</li><li>`project:deleted`</li></ul>
| [`public`](https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#public) | specific repository || <ul><li>`public`</li></ul>
| [`pull_request`](https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#pull_request) | specific commit | <ul><li>`opened`</li><li>`edited`</li><li>`closed`</li><li>`assigned`</li><li>`unassigned`</li><li>`review_requested`</li><li>`review_request_removed`</li><li>`ready_for_review`</li><li>`converted_to_draft`</li><li>`labeled`</li><li>`unlabeled`</li><li>`synchronize`</li><li>`auto_merge_enabled`</li><li>`auto_merge_disabled`</li><li>`locked`</li><li>`unlocked`</li><li>`reopened`</li></ul> | <ul><li>`pull_request:opened`</li><li>`pull_request:edited`</li><li>`pull_request:closed`</li><li>`pull_request:assigned`</li><li>`pull_request:unassigned`</li><li>`pull_request:review_requested`</li><li>`pull_request:review_request_removed`</li><li>`pull_request:ready_for_review`</li><li>`pull_request:converted_to_draft`</li><li>`pull_request:labeled`</li><li>`pull_request:unlabeled`</li><li>`pull_request:synchronize`</li><li>`pull_request:auto_merge_enabled`</li><li>`pull_request:auto_merge_disabled`</li><li>`pull_request:locked`</li><li>`pull_request:unlocked`</li><li>`pull_request:reopened`</li></ul>
| [`pull_request_review`](https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#pull_request_review) | specific commit | <ul><li>`submitted`</li><li>`edited`</li><li>`dismissed`</li></ul> | <ul><li>`pull_request_review:submitted`</li><li>`pull_request_review:edited`</li><li>`pull_request_review:dismissed`</li></ul>
| [`pull_request_review_comment`](https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#pull_request_review_comment) | specific commit | <ul><li>`created`</li><li>`edited`</li><li>`deleted`</li></ul> | <ul><li>`pull_request_review_comment:created`</li><li>`pull_request_review_comment:edited`</li><li>`pull_request_review_comment:deleted`</li></ul>
| [`push`](https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#push) | specific commit || <ul><li>`push`</li></ul>
| [`release`](https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#release) | specific repository | <ul><li>`published`</li><li>`unpublished`</li><li>`created`</li><li>`edited`</li><li>`deleted`</li><li>`prereleased`</li><li>`released`</li></ul> | <ul><li>`release:published` + ⭐️&nbsp;&nbsp;`cd:pipeline_requested`</li><li>`release:unpublished`</li><li>`release:created`</li><li>`release:edited`</li><li>`release:deleted`</li><li>`release:prereleased`</li><li>`release:released`</li></ul>
| [`repository`](https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#repository) | specific repository | <ul><li>`created`</li><li>`deleted`</li><li>`archived`</li><li>`unarchived`</li><li>`anonymous_access_enabled`</li><li>`edited`</li><li>`renamed`</li><li>`transferred`</li><li>`publicized`</li><li>`privatized`</li></ul> | <ul><li>`repository:created`</li><li>`repository:deleted`</li><li>`repository:archived`</li><li>`repository:unarchived`</li><li>`repository:anonymous_access_enabled`</li><li>`repository:edited`</li><li>`repository:renamed`</li><li>`repository:transferred`</li><li>`repository:publicized`</li><li>`repository:privatized`</li></ul>
| [`status`](https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#status) | specific commit || <ul><li>`status`</li></ul>
| [`team_add`](https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#team_add) | specific repository || <ul><li>`team_add`</li></ul>
| [`watch`](https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#watch) | specific repository | <ul><li>`started`</li></ul> | <ul><li>`watch:started`</li></ul>
