# Implementing CI/CD with Brigade and the GitHub Gateway

In this section, we'll explore how this gateway can easily be used to implement
CI/CD pipelines.

We do begin with a fair bit of background. If you're in a hurry, you might want
to skip to the [CI/CD Recipe](#cicd-recipe) section.

## The GitHub Checks API

When implementing CI/CD, the GitHub
[Checks API](https://docs.github.com/en/rest/reference/checks) is of particular
significance. A _check_ is some assertion that can be made upon a code base.
Examples would include executing a battery of tests or subjecting code to some
kind of static analysis such as linting rules. A _check suite_, as one can infer
from the name, is a _collection_ of checks.

Provided you subscribed to them when you set up your
[GitHub App](https://docs.github.com/en/developers/apps/about-apps), GitHub will
send a `check_suite` webhook with action `requested` to your gateway anytime new
commits are pushed _directly_ to a repository into which your GitHub App is
installed. This intuitively makes good sense: _When GitHub becomes aware of new
code, it requests for that code be validated._

## Check Suite Forwarding

Often, however, code is not committed _directly_ to a given repository.
Typically, pull requests are opened instead, with a source branch belonging to a
_fork_ since most projects, rightfully, do not permit untrusted contributors to
push code directly to their repository. For such cases, GitHub does _not_
automatically send a `check_suite` webhook to your gateway. There is sound
rationale for this: If a contributor lacks permissions to push code directly to
a given repository, then their contributions cannot be trusted to an extent that
they can safely be tested automatically. After all, malicious modifications to
the code or tests could put a software project at great risk. Imagine, for
instance, if any random person on the internet could hijack your CI pipelines to
steal project secrets or mine crypto coins.

To recap: New PRs with a source branch belonging to a fork do _not_
automatically result in `check_suite` webhooks being sent to your gateway. In
such cases, however, provided you subscribed to them when you set up your
GitHub App, `pull_request` webhooks with action `opened` _are_ sent.

When this gateway receives a `pull_request` webhook with a value of `opened`,
`reopened`, or `synchronized` in the JSON payload's `action` field, it further
scrutinizes the JSON payload to determine the PR author's relationship to the
target repository. If the author is determined to be a trusted contributor (an
`OWNER` of the repository, for instance), the gateway uses the GitHub Checks API
to create a check suite and request that a `check_suite` webhook with action
`rerequested` be sent along to the gateway. We call this process _check suite
forwarding_. If the PR author is determined _not_ to be a trusted contributor,
then this does _not_ occur.

> ⚠️&nbsp;&nbsp;For check suite forwarding to work for new/updated PRs your
> GitHub App should be subscribed to `pull_request` webhooks, but there is no
> need for Brigade projects to subscribe to `pull_request:opened`,
> `pull_request:reopened`, or `pull_request:synchronized` events. Check suite
> forwarding is purely a function of the gateway and individual projects do not
> need to do anything to enable it.

In cases where no check suite forwarding occurred, a trusted contributor may
review the PR and, if they deem it safe, can comment either `/brig run` or
`/brig check`. Provided you subscribed to them when you set up your GitHub App,
this results in an `issue_comment` webhook with action `created` being sent to
the gateway.

The check suite forwarding process described above for `pull_request` webhooks
also applies to `issue_comment` webhooks. If an `issue_comment` webhook with an
`action` value of `created` is received by this gateway and scrutiny of the
webhook's JSON payload reveals the comment author is a trusted contributor, then
the check suite forwarding process proceeds as if the comment author had
authored the PR themselves.

> ⚠️&nbsp;&nbsp;For check suite forwarding to work for `/brig run` or `/brig
> check` comments, your GitHub App should be subscribed to `issue_comment`
> webhooks, but there is no need for Brigade projects to subscribe to
> `issue_comment:created` events. Check suite forwarding is purely a function of
> the gateway and individual projects do not need to do anything to enable it.

## Check Results

In any cases where the gateway emits a `check_suite:requested` or
`check_suite:rerequested` event into Brigade's event bus (regardless of whatever
check suite forwarding may or may not have been involved in getting to that
point), the gateway will also monitor the status of all jobs associated with
those events and utilize the Checks API to report results back to GitHub. In
this way, the result of every such job becomes the result of a single
corresponding check in the corresponding check suite. This is how Brigade job
results and logs become viewable in the GitHub web UI.

In the event that any individual job fails, the corresponding check can be
re-run by an authorized user via GitHub's web UI. This results in a `check_run`
webhook with action `rerequested` being sent to the gateway and a
`check_run:requested` event being emitted into Brigade's event bus. This permits
Brigade projects to subscribe to and handle requests to re-run a single job.

## Releases

When an authorized user creates a new release or makes a release draft public
using the GitHub web UI, and provided you subscribed to it when setting up your
GitHub App, GitHub will send a `release` webhook with action `published` to your
gateway. Brigade projects may wish to subscribe to the corresponding
`release:published` event to trigger their continuous delivery/deployment
pipelines.

## Custom CI/CD Events

For convenience, Brigade emits a _second_ event for many of the scenarios
discussed above. The intent behind this is to _distill_ the many nuanced details
of those scenarios into a small and consistently named set of event types that
can be readily understood.

Since script authors rarely, if ever, need to differentiate between a
`check_suite:requested` event and a `check_suite:rerequested` event, when
emitting either of these into Brigade's event bus, this gateway _also_ emits a
`ci_pipeline:requested` event. Apart from effectively collapsing two similarly
named and nearly identical events into one, the name `ci_pipeline:requested`
very clearly denotes exactly what any subscribed project's script should do to
handle such an event -- namely, run the CI pipeline. Better still, it eliminates
any potential confusion arising from questions like, "What even is a check
suite?" Since the gateway can handle such things all on its own, it is perhaps
better for Brigade's end-users not to get bogged down in such details and simply
focus on the fact that a `ci_pipeline:requested` event means CI should run.

Again, to help end-users avoid getting bogged down in the complexities of things
such as the GitHub Checks API, when emitting a `check_run:rerequested` event,
this gateway will _also_ emit a `ci_job:requested`. Again, this name clearly
denotes exactly what any subscribed project's script should do to handle such an
event -- namely, run some discreet segment of the CI pipeline. As an added
convenience, `ci_job:requested` events have a `job` label that indicates _which_
specific job is to be re-run. This spares script authors from digging into the
event payload to make this determination.

Last, and primarily for consistency with the `ci_pipeline:requested` and
`ci_job:requested` names, when emitting a `release:published` event, this
gateway will _also_ emit a `cd_pipeline:requested` event. Once again, this name
clearly denotes exactly what any subscribed project's script should do in
response to such an event. As an added convenience, `cd_pipeline:requested` have
a `release` label that indicates the release name. This spares script authors
from inferring this information themselves by digging into the event payload or
other event details.

> ⚠️&nbsp;&nbsp;These custom events are emitted _in addition to_ the original
> events; not _instead of_. This preserves the flexibility for script authors to
> "drop down" to the original events if/when necessary. Script authors should
> take care not to subscribe to the original events _and_ their corresponding
> custom events, as the net effect would be that every singular _logical_ event
> would be received and processed twice.

To summarize, the existence of the custom events discussed in this section
means that script authors who are concerned only with CI/CD need only concern
themselves with the following three events:

* `ci_pipeline:requested`
* `ci_job:requested`
* `cd_pipeline:requested`

## CI/CD Recipe

This section presents a reliable CI/CD recipe for Brigade and the GitHub
Gateway.

### Project Definition:

TODO: Still need some test here to introduce the example:

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
    - ci_pipeline:requested
    - ci_job:requested
    - cd_pipeline:requested
  workerTemplate:
    git:
      cloneURL: https://github.com/brigadecore/ci-cd-example.git
```

### Script

TODO: Still need some test here to introduce the example:

```typescript
import { events, Event, Job, ConcurrentGroup, Container } from "@brigadecore/brigadier"

events.on("brigade.sh/github", "ci_pipeline:requested", async event => {
  // Chain some jobs together to implement CI. For example:
  await new ConcurrentGroup(
    // For brevity, we're omitting the definitions of each job.
    testJob0,
    testJob1,
    // ...,
    testJobN
  ).run()
})

events.on("brigade.sh/github", "cd_pipeline:requested", async event => {
  // Chain some jobs together to implement CD. For example:
  await new ConcurrentGroup(
    // For brevity, we're omitting the definitions of each job.
    releaseJob0,
    releaseJob1,
    // ...,
    releaseJobN,
  ).run()
}

events.process()
```

Unaccounted for in the script above are `ci_job:requested` events that indicate
that a specific job should be re-run. Modifying the previous script slightly,
we can account for such events. The strategy makes use of a map of job factory
functions indexed by name:

```typescript
import { events, Event, Job, ConcurrentGroup, Container } from "@brigadecore/brigadier"

// A map of job factory functions indexed by name. When a ci_job:requested
// event wants to re-run a single job, this allows us to easily find it.
const jobs: {[key: string]: (event: Event) => Job } = {}

const testJob0Name = "testJob0"
const testJob0 = (event: Event) => {
  return new Job(testJob0Name, "some/image:tag", event)
}
jobs[testJob0Name] = testJob0

// Remaining job factory function definitions are omitted for brevity

// ...

events.on("brigade.sh/github", "ci_pipeline:requested", async event => {
  // Chain some jobs together to implement CI. For example:
  await new ConcurrentGroup(
    testJob0(event),
    testJob1(event),
    // ...,
    testJobN(event)
  ).run()
})

events.on("brigade.sh/github", "ci_job:requested", async event => {
  // The job name can be found in a label
  const job = jobs[event.labels.job]
  if (job) {
    await job(event).run()
    return
  }
  throw new Error(`No job found with name: ${event.labels.job}`)
})

events.on("brigade.sh/github", "cd_pipeline:requested", async event => {
  // Chain some jobs together to implement CD. For example:
  await new ConcurrentGroup(
    releaseJob0(event),
    releaseJob1(event),
    // ...,
    releaseJobN(event),
  ).run()
}

events.process()
```
