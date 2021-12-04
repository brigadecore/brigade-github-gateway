# Brigade Github Gateway

![build](https://badgr.brigade2.io/v1/github/checks/brigadecore/brigade-github-gateway/badge.svg?appID=99005)
[![codecov](https://codecov.io/gh/brigadecore/brigade-github-gateway/branch/main/graph/badge.svg?token=ZPY3OF13FC)](https://codecov.io/gh/brigadecore/brigade-github-gateway)
[![Go Report Card](https://goreportcard.com/badge/github.com/brigadecore/brigade-github-gateway)](https://goreportcard.com/report/github.com/brigadecore/brigade-github-gateway)
[![slack](https://img.shields.io/badge/slack-brigade-brightgreen.svg?logo=slack)](https://kubernetes.slack.com/messages/C87MF1RFD)

<img width="100" align="left" src="logo.png">

This is a work-in-progress
[Brigade 2](https://github.com/brigadecore/brigade/tree/v2)
compatible gateway that can be used to receive events (webhooks) from one or
more [GitHub Apps](https://docs.github.com/en/developers/apps/about-apps)
and propagate them into Brigade 2's event bus.

<br clear="left"/>

## Installation

The installation for this gateway is multi-part, and not particularly easy, at
least in part because of a potential "chicken and egg" problem. Setting up this
gateway requires a value obtained during the creation of a GitHub App. Setting
up the GitHub App _may_ require the gateway's public IP (if you're not using a
domain or subdomain name instead). We will use an approach of setting up the
GitHub App first, with a placeholder value for the gateway's address, if
necessary, in which case the GitHub App configuration will be revisited after
the gateway is configured and deployed.

Prerequisites:

* A GitHub account

* A Kubernetes cluster:
    * For which you have the `admin` cluster role
    * That is already running Brigade 2
    * Capable of provisioning a _public IP address_ for a service of type
      `LoadBalancer`. (This means you won't have much luck running the gateway
      locally in the likes of kind or minikube unless you're able and willing to
      mess with port forwarding settings on your router, which we won't be
      covering here.)

* `kubectl`, `helm` (commands below assume Helm 3.7.0+), and `brig` (the Brigade
  2 CLI)

If you want to avoid the aforementioned "chicken and egg" problem, reserving a
domain or subdomain name (for which you control DNS) also helps. If you don't
want to do this or are unable, we'll cover that scenario as well.

### 1. Create a GitHub App

A [GitHub App](https://docs.github.com/en/developers/apps/about-apps) is a
special kind of trusted entity that is "installable" into GitHub repositories to
enable integrations.

This gateway can support multiple GitHub Apps, but these instructions walk you
through the steps for setting up just one.

* Visit https://github.com/settings/apps/new.

* Choose a _globally unique_ __GitHub App name__.
    * When you submit the form, you'll be informed if the name you selected is
      unavailable.

* Set the __Homepage URL__ to
  `https://github.com/brigadecore/brigade-github-gateway`. Really, any URL will
  work, but this is the URL to which users will be directed if they wish to know
  more information about the App, so something informative is best. The URL
  above links to what is presently the best source of information about this
  gateway.

* Set the __Webhook URL__ to 
  `https://<your gateway domain or subdomain name>/events`.
    * If you're not using a domain or subdomain name and want to use a public IP
      here instead, put a placeholder such as `http://example.com/events` here
      for now and revisit this section later after a public IP has been
      established for your gateway.

* Set the __Webhook Secret__ to a complex string. It is, fundamentally, a
  password, so make it strong. If you're personally in the habit of using a password
  manager and it can generate strong passwords for you, consider using that.
  Make a note of this __shared secret__. You will be using this value again in
  another step.

* Under the __Subscribe to events__ section, select any events you wish to
  propagate to Brigade.
    * Note: Selecting additional permissions in the __Repository permissions__
      section adds additional options to the menu of subscribable events.
    * For the examples that follow, you would require the __Watching__
      permission and a subscription to the __Watch__ event.

* Unless you want anyone on GitHub to be able to send events to your gateway,
  toward the bottom of the page, select __Only This account__ to constrain your
  GitHub App to being installed only by repositories in your own account or
  organization. If you select __Any account__ instead, be sure you know what
  you're doing!

* Submit the form.

After submitting the form, find the __App ID__ field and take note. You will be
using this value again in another step.

Under the `Private keys` section of this page, click `Generate a private key`.
After generating, immediately download the new key. _It is your only opportunity
to do so, as GitHub will only save the public half of the key. You will be using
this key in another step._

### 2. Create a Service Account for the Gateway

__Note:__ To proceed beyond this point, you'll need to be logged into Brigade 2
as the "root" user (not recommended) or (preferably) as a user with the `ADMIN`
role. Further discussion of this is beyond the scope of this documentation.
Please refer to Brigade's own documentation.

Using Brigade 2's `brig` CLI, create a service account for the gateway to use:

```console
$ brig service-account create \
    --id brigade-github-gateway \
    --description brigade-github-gateway
```

Make note of the __token__ returned. This value will be used in another step.
_It is your only opportunity to access this value, as Brigade does not save it._

Authorize this service account to read all events and to create new ones:

```console
$ brig role grant READER \
    --service-account brigade-github-gateway

$ brig role grant EVENT_CREATOR \
    --service-account brigade-github-gateway \
    --source brigade.sh/github
```

__Note:__ The `--source brigade.sh/github` option specifies that this service
account can be used _only_ to create events having a value of
`brigade.sh/github` in the event's `source` field. _This is a security measure
that prevents the gateway from using this token for impersonating other gateways._

### 3. Install the GitHub Gateway

For now, we're using the [GitHub Container Registry](https://ghcr.io) (which is
an [OCI registry](https://helm.sh/docs/topics/registries/)) to host our Helm
chart. Helm 3.7 has _experimental_ support for OCI registries. In the event that
the Helm 3.7 dependency proves troublesome for Brigade users, or in the event that
this experimental feature goes away, or isn't working like we'd hope, we will
revisit this choice before going GA.

First, be sure you are using
[Helm 3.7.0](https://github.com/helm/helm/releases/tag/v3.7.0) or greater and
enable experimental OCI support:

```console
$ export HELM_EXPERIMENTAL_OCI=1
```

As this chart requires custom configuration as described above to function
properly, we'll need to create a chart values file with said config.

Use the following command to extract the full set of configuration options into
a file you can modify:

```console
$ helm inspect values oci://ghcr.io/brigadecore/brigade-github-gateway \
    --version v0.4.0 > ~/brigade-github-gateway-values.yaml
```

Edit `~/brigade-github-gateway-values.yaml`, making the following changes:

* `brigade.apiAddress`: Address of the Brigade API server, beginning with
  `https://`

* `brigade.apiToken`: Service account token from step 2

* `github.apps`: Specify the details of your GitHub App(s), including:

    * `appID`: App ID from step 1

    * `apiKey`: The private key downloaded in step 1, beginning
      with `-----BEGIN RSA PRIVATE KEY-----` and ending with
      `-----END RSA PRIVATE KEY-----`. All line breaks should be preserved and
      the beginning of each line should be indented exactly four spaces.

    * `sharedSecret`: Shared secret from step 1

* `receiver.host`: Set this to the host name where you'd like the gateway to be
  accessible.

* `receiver.service.type`: If you plan to enable ingress (advanced), you can
  leave this as its default -- `ClusterIP`. If you do not plan to enable
  ingress, you probably will want to change this value to `LoadBalancer`.

Save your changes to `~/brigade-github-gateway-values.yaml` and use the
following command to install the gateway using the above customizations:

```console
$ helm install brigade-github-gateway \
    oci://ghcr.io/brigadecore/brigade-github-gateway \
    --version v0.4.0 \
    --create-namespace \
    --namespace brigade-github-gateway \
    --values ~/brigade-github-gateway-values.yaml \
    --wait \
    --timeout 300s
```

### 4. (RECOMMENDED) Create a DNS Entry

In the prerequisites section, we suggested that you reserve a domain or
subdomain name as the address of your gateway. At this point, you should be able
to associate that name with the gateway's public IP address.

If you overrode defaults and set `service.type` to `LoadBalancer`, use this
command to find the gateway's public IP address:

```console
$ kubectl get svc brigade-github-gateway-receiver \
    --namespace brigade-github-gateway \
    --output jsonpath='{.status.loadBalancer.ingress[0].ip}'
```

If you overrode defaults and enabled support for an ingress controller, you
probably know what you're doing well enough to track down the correct IP without
our help. 😉

With this public IP in hand, edit your name servers and add an `A` record
pointing your domain to the public IP.

__Note:__ If you do not want to use a domain or subdomain name, or are unable
to, and elected to use a placeholder URL when initially setting up your GitHub
App, return to GitHub (your App can be found on GitHub using a URL of the form
`https://github.com/settings/apps/<app name>`) and edit your App's configuration
to send webhooks (events) to `https://<public ip>/events`.

### 5. Confirm Connectivity

Your App can be found on GitHub using a URL of the form
`https://github.com/settings/apps/<app name>`.

Go to the __Advanced__ tab and check out the __Recent Deliveries__ section. Here
you can view events that your GitHub App has recently attempted to deliver to
your new gateway. There shouldn't be many events displayed yet, but there should
be at least one `ping` event that the App attempted to deliver to the gateway
when the App was created. This should have failed since we set up the App on
GitHub's end _prior_ to installing the gateway on our cluster. Click
__Redeliver__. If re-delivery succeeds, you're all set!

If re-delivery failed, you can examine request and response headers and payload
to attempt to make some determination as to what has gone wrong.

Some likely problems include:

* Your A record in DNS is incorrect.

* DNS changes have not propagated.

* Your gateway is not listening on a public IP.

* The __Webhook URL__ you entered when configuring the GitHub App is incorrect.

* The gateway was not configured correctly using the GitHub App's __App ID__
  and __shared secret__.

### 6. Install the App

Your App can be found on GitHub using a URL of the form
`https://github.com/settings/apps/<app name>`.

Under the __Install App__ tab you can see all accounts and organizations into
whose repositories you can install your App. Click the gear icon next to the
desired account or organization and, under __Repository access__ choose __All
repositories__ OR __Only select repositories__ then specify which ones, and
click __Save__.

### 7. Add a Brigade Project

You can create any number of Brigade projects (or modify an existing one) to
listen for events sent from your GitHub App to your gateway and, in turn,
emitted into Brigade's event bus. You can subscribe to all event types emitted
by the gateway, or just specific ones.

In the example project definition below, we subscribe to all events emitted by
the gateway, provided they've originated from the `example-org/example-repo`
repository (see the `repo` qualifier). You should adjust this value to match a
repository into which you have installed your new GitHub App.

```yaml
apiVersion: brigade.sh/v2
kind: Project
metadata:
  id: github-demo
description: A project that demonstrates integration with GitHub
spec:
  eventSubscriptions:
  - source: brigade.sh/github
    types:
    - *
    qualifiers:
      repo: example-org/example-repo
  workerTemplate:
    defaultConfigFiles:
      brigade.js: |-
        const { events } = require("@brigadecore/brigadier");

        events.on("brigade.sh/github", "watch:started", () => {
          console.log("Someone starred the example-org/example-repo repository!");
        });

        events.process();
```

In the alternative example below, we subscribe _only_ to `watch:started` events.
(Note that, counterintuitively, this event occurs when someone _stars_ a
repository; not when they start watching it. This is a peculiarity of GitHub and
not a peculiarity of this gateway.)

```yaml
apiVersion: brigade.sh/v2
kind: Project
metadata:
  id: github-demo
description: A project that demonstrates integration with GitHub
spec:
  eventSubscriptions:
  - source: brigade.sh/github
    types:
    - watch:started
    qualifiers:
      repo: example-org/example-repo
  workerTemplate:
    defaultConfigFiles:
      brigade.js: |-
        const { events } = require("@brigadecore/brigadier");

        events.on("brigade.sh/github", "watch:started", () => {
          console.log("Someone starred the example-org/example-repo repository!");
        });

        events.process();
```

Assuming this file were named `project.yaml`, you can create the project like
so:

```console
$ brig project create --file project.yaml
```

Adding a star to the repo into which you installed your new GitHub App should
now send an event (webhook) from GitHub to your gateway. The gateway, in turn,
will emit the event into Brigade's event bus. Brigade should initialize a worker
(containerized event handler) for every project that has subscribed to the
event, and the worker should execute the `brigade.js` script that was embedded
in the project definition.

List the events for the `github-demo` project to confirm this:

```console
$ brig event list --project github-demo
```

Full coverage of `brig` commands is beyond the scope of this documentation, but
at this point, additional `brig` commands can be applied to monitor the event's
status and view logs produced in the course of handling the event.

## Events Received and Emitted by this Gateway

Events received by this gateway from GitHub are, in turn, emitted into
Brigade's event bus.

Events of certain types received from GitHub are further qualified by the value
of an `action` field. In all such cases, the event emitted into Brigade's event
bus will have a type of the form `<original event type>:<action>`. For instance,
if this gateway receives an event of type `pull_request` from GitHub with the
value `opened` in the `action` field, the event emitted into Brigade's event bus
will be of type `pull_request:opened`.

Events received from GitHub vary in _scope of specificity_. All events handled
by this gateway are _at least_ indicative of activity involving some specific
repository in some way -- for instance, a GitHub user having starred or forked a
repository. Some events, however, are more specific than this, being indicative
of activity involving not only a specific repository, but also some specific
branch, tag, or commit -- for instance, a new pull request has been opened or a
new tag has been pushed. In such cases (and only in such cases), this gateway
includes git reference or commit information in the event that is emitted into
Brigade's event bus. By doing so, Brigade (which has built in _git_ support;
_not GitHub support_) is enabled to locate specific code affected by the event.

If the gateway is able to infer a human-friendly title for any event, the event
emitted into Brigade's event bus is augmented with this information.

The following table summarizes all GitHub event types that can be received by
this gateway and the corresponding event types that are emitted into Brigade's
event bus.

| GitHub Event Type | Scope | Possible Action Values | Event Type(s) Emitted |
|-------------------|-------|------------------------|-----------------------|
| [`check_run`](https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#check_run) | specific commit | <ul><li>`created`</li><li>`completed`</li><li>`rerequested`</li><li>`rerequested_action`</li></ul> | <ul><li>`check_run:created`</li><li>`check_run:completed`</li><li>`check_run:rerequested`</li><li>`check_run:rerequested_action`</li></ul>
| [`check_suite`](https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#check_suite) | specific commit | <ul><li>`completed`</li><li>`requested`</li><li>`rerequested`</li></ul> | <ul><li>`check_suite:completed`</li><li>`check_suite:requested`</li><li>`check_suite:rerequested`</li></ul>
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
| [`release`](https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#release) | specific repository | <ul><li>`published`</li><li>`unpublished`</li><li>`created`</li><li>`edited`</li><li>`deleted`</li><li>`prereleased`</li><li>`released`</li></ul> | <ul><li>`release:published`</li><li>`release:unpublished`</li><li>`release:created`</li><li>`release:edited`</li><li>`release:deleted`</li><li>`release:prereleased`</li><li>`release:released`</li></ul>
| [`repository`](https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#repository) | specific repository | <ul><li>`created`</li><li>`deleted`</li><li>`archived`</li><li>`unarchived`</li><li>`anonymous_access_enabled`</li><li>`edited`</li><li>`renamed`</li><li>`transferred`</li><li>`publicized`</li><li>`privatized`</li></ul> | <ul><li>`repository:created`</li><li>`repository:deleted`</li><li>`repository:archived`</li><li>`repository:unarchived`</li><li>`repository:anonymous_access_enabled`</li><li>`repository:edited`</li><li>`repository:renamed`</li><li>`repository:transferred`</li><li>`repository:publicized`</li><li>`repository:privatized`</li></ul>
| [`status`](https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#status) | specific commit || <ul><li>`status`</li></ul>
| [`team_add`](https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#team_add) | specific repository || <ul><li>`team_add`</li></ul>
| [`watch`](https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#watch) | specific repository | <ul><li>`started`</li></ul> | <ul><li>`watch:started`</li></ul>

## Scripting Guide

In this section, we'll explore a popular use case for this gateway -- CI/CD.

### Overview

When implementing CI/CD, the GitHub
[checks API](https://docs.github.com/en/rest/reference/checks) is of particular
significance. A _check_ is some assertion that can be made upon a
code base. Examples would include executing a battery of unit tests or
subjecting code to some kind of static analysis such as linting rules. A _check
suite_, as one can infer from the name, is a _collection_ of checks.

Provided you subscribed your gateway to them when you set up the corresponding
GitHub App, GitHub will forward a `check_suite` webhook with action `requested`
to your gateway anytime new commits are pushed _directly_ to an applicable
repository. This intuitively makes good sense. _When GitHub becomes aware of new
code, it requests that code be validated._

Often, however, workflows involve pull requests wherein the source branch
belongs to a _fork_ since most projects, rightfully, do not permit
non-maintainers to push branches or tags directly to their repository. For such
cases, no `check_suite` web hook is forwarded from GitHub to your gateway. There
is sound rationale for this. If a contributor lacks adequate permissions to push
code directly to a given repository, then it follows that their contributions
can not be implicitly trusted to an extent that they can safely be tested
automatically -- since malicious modifications to the code or tests could put a
software project at risk. (For instance, imagine if any random person on the
internet could hijack your CI processes to steal project secrets or mine crypto
coins.)

So, generally speaking, PRs do _not_ automatically result in `check_suite` web
hooks being forwarded to your gateway. In such cases, however, provided you
subscribed when setting up the GitHub App, of course, `pull_request` web hooks
_are_ forwarded.

By default, the GitHub gateway _intercepts_ `pull_request` web hooks with
actions `opened`, `reopened`, or `synchronized` and scrutinizes the PR author's
relationship to the target repository. If the author is determined to be a
trusted contributor, the gateway uses the checks API to explicitly ask GitHub to
forward a corresponding `check_suite` webhook with action `rerequested`. If the
author is determined _not_ to be a trusted contributor, then this does _not_
happen. In such cases, a trusted contributor may review the PR and, if they deem
it safe, can comment `/brig run`. This results in an `issue_comment` webhook
being sent to the gateway. These also are intercepted and if/when the
commentor's status as a trusted contributor is verified, the gateway utilizes
the checks API to to explicitly ask GitHub to forward a corresponding
`check_suite` webhook with action `rerequested` -- just the same as if the
reviewer had authored the PR themselves.

In any case where the gateway forwards a `check_suite:rerequested` event to
Brigade to be enqueued for any subscribed project, the gateway will also monitor
the status of any and all jobs associated with each event and utilize the checks
API to return test results to GitHub. In this way, the result of every job
becomes the result of a single check (having the same name as the job) in the
corresponding check suite. (This is how job results end up visible in the GitHub
web UI.)

In the event that any individual check in the suite fails, it can be
re-requested by an authorized user via GitHub's web UI. This results in a web
hook of type `check_run` with action `rerequested` being forwarded to the
gateway.

__If all of the above is confusing, do not dismay.__ While the GitHub checks API
is complex and this gateway's interaction with it is rife with technical
nuances, the bottom line is quite simple: If you are interested in implementing
CI/CD using Brigade and this gateway, your Brigade projects should subscribe to
the following events:

* `check_suite:rerequested`: This is a reliable indicator that a PR has been
  opened or updated, the gateway has intercepted that event and determined it is
  safe to test, _or_ a maintainer has made that determination and requested for
  tests to proceed by commenting `/brig run`.
* `check_suite:requested`: This is a reliable indicator that new code has been
  pushed directly to the repository or a PR has been merged into its target
  branch.
* `check_run:rerequested`: This is a reliable indicator that an authorized
  user has requested a specific check (i.e. job) to be re-run.
* `push`: This is a reliable indicator that new code has been pushed directly to
  the repository, a PR has been merged into its target branch, _or a new tag has
  been pushed to the repository_. It is this last condition that we will
  generally be concerned with since the other conditions _also_ result in
  `check_run:rerequested` or `check_run:requested` events.

A project definition that is subscribed to all of the above may then resemble
this one:

```yaml
apiVersion: brigade.sh/v2
description: A CI/CD example
kind: Project
metadata:
  id: ci-cd-example
spec:
  eventSubscriptions:
  - source: brigade.sh/github
    qualifiers:
      repo: brigadecore/ci-cd-example
    types:
    - check_run:rerequested
    - check_suite:requested
    - check_suite:rerequested
    - push
  workerTemplate:
    git:
      cloneURL: https://github.com/brigadecore/ci-cd-example.git
```

### Responding with Script

If you've used a project definition similar to the one in the previous section,
the events discussed in that section will trigger a worker that will execute
your script. It is still up to your script to distinguish between different
scenarios and handle them accordingly. The following TypeScript (`brigade.ts`)
demonstrates a common pattern. Of course, you can use this as a starting point
and adapt it to suit your needs:

```typescript
import { events, Event, Job, ConcurrentGroup, Container } from "@brigadecore/brigadier"

async function runSuite(event: Event): Promise<void> {
  // Chain some jobs together to implement CI. For example:
  await new ConcurrentGroup(
    // For brevity, we're omitting the definitions of each job.
    testJob0,
    testJob1,
    // ...,
    testJobN
  ).run()
}

// Either of these events should initiate execution of the entire test suite,
// so we've broken that test suite out into a reusable function.
events.on("brigade.sh/github", "check_suite:requested", runSuite)
events.on("brigade.sh/github", "check_suite:rerequested", runSuite)

// Pushing (or merging) commits to any branch in github triggers a check suite.
// Such events are already handled above. So here, we're ONLY concerned with the
// case wherein a new TAG has been pushed. In this example at least, we're only
// concerned with tags that honor semantic versioning and therefore indicate
// a RELEASE.
events.on("brigade.sh/github", "push", async event => {
  const releaseTagRegex = /^refs\/tags\/(v[0-9]+(?:\.[0-9]+)*(?:\-.+)?)$/
  const matchStr = event.worker.git.ref.match(releaseTagRegex)
  if (matchStr) {
    // Chain some jobs together to implement CD. For example:
    await new ConcurrentGroup(
      // For brevity, we're omitting the definitions of each job.
      releaseJob0,
      releaseJob1,
      // ...,
      releaseJobN,
    ).run()
  } else {
    console.log(`Ref ${event.worker.git.ref} does not match release tag regex (${releaseTagRegex}); nothing to run.`)
  } 
}

// Dispatch the event
events.process()
```

Unaccounted for in the script above are `check_run:rerequested` events that
indicate that a specific check (i.e. a specific job), should be re-run.
Modifying the previous script slightly, we can account for such events. The
strategy makes us of a map of job factory functions indexed by name:

```typescript
import { events, Event, Job, ConcurrentGroup, Container } from "@brigadecore/brigadier"

// A map of job factory functions indexed by name. When a check_run:rerequested
// event wants to re-run a single job, this allows us to easily accommodate
// that.
const jobs: {[key: string]: (event: Event) => Job } = {}

const testJob0Name = "testJob0"
const testJob0 = (event: Event) => {
  return new Job(testJob0Name, "some/image:tag", event)
}
jobs[testJob0Name] = testJob0

// Remaining job factory function definitions are omitted for brevity

// ...

async function runSuite(event: Event): Promise<void> {
  // Chain some jobs together to implement CI. For example:
  await new ConcurrentGroup(
    testJob0(event),
    testJob1(event),
    // ...,
    testJobN(event)
  ).run()
}

// Either of these events should initiate execution of the entire test suite,
// so we've broken that test suite out into a reusable function.
events.on("brigade.sh/github", "check_suite:requested", runSuite)
events.on("brigade.sh/github", "check_suite:rerequested", runSuite)

// This event indicates a specific job is to be re-run.
events.on("brigade.sh/github", "check_run:rerequested", async event => {
  // Check run names are of the form <project name>:<job name>, so we strip
  // event.project.id.length + 1 characters off the start of the check run name
  // to find the job name.
  const jobName = JSON.parse(event.payload).check_run.name.slice(event.project.id.length + 1)
  const job = jobs[jobName]
  if (job) {
    await job(event).run()
    return
  }
  throw new Error(`No job found with name: ${jobName}`)
})

// Pushing (or merging) commits to any branch in github triggers a check suite.
// Such events are already handled above. So here, we're ONLY concerned with the
// case wherein a new TAG has been pushed. In this example at least, we're only
// concerned with tags that honor semantic versioning and therefore indicate
// a RELEASE.
events.on("brigade.sh/github", "push", async event => {
  const releaseTagRegex = /^refs\/tags\/(v[0-9]+(?:\.[0-9]+)*(?:\-.+)?)$/
  const matchStr = event.worker.git.ref.match(releaseTagRegex)
  if (matchStr) {
    // Chain some jobs together to implement CD. For example:
    await new ConcurrentGroup(
      releaseJob0(event),
      releaseJob1(event),
      // ...,
      releaseJobN(event),
    ).run()
  } else {
    console.log(`Ref ${event.worker.git.ref} does not match release tag regex (${releaseTagRegex}); nothing to run.`)
  } 
}

// Dispatch the event
events.process()
```

### Simplifying

While the project maintainers readily acknowledge that the CI/CD implementation
pattern demonstrated across the previous two sections is complex, it is, at the
very least tried and true. The maintainers have applied this pattern to Brigade
itself, this gateway, and dozens of other Brigade-related projects. Still, we
know improvement is possible.

Plans are already underway to distill this pattern into something more succinct
and more readily reusable. Expect to see such a solution included very soon in a
minor release.

For reference, please refer to
[this issue](https://github.com/brigadecore/brigade-github-gateway/issues/4).

## Contributing

The Brigade project accepts contributions via GitHub pull requests. The
[Contributing](CONTRIBUTING.md) document outlines the process to help get your
contribution accepted.

## Support & Feedback

We have a slack channel!
[Kubernetes/#brigade](https://kubernetes.slack.com/messages/C87MF1RFD) Feel free
to join for any support questions or feedback, we are happy to help. To report
an issue or to request a feature open an issue
[here](https://github.com/brigadecore/brigade/issues)

## Code of Conduct

Participation in the Brigade project is governed by the
[CNCF Code of Conduct](https://github.com/cncf/foundation/blob/master/code-of-conduct.md).
