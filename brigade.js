// ============================================================================
// NOTE: This is a Brigade 1.x script for now.
// ============================================================================

const { events, Job } = require("brigadier");
const { Check } = require("@brigadecore/brigade-utils");

const releaseTagRegex = /^refs\/tags\/(v[0-9]+(?:\.[0-9]+)*(?:\-.+)?)$/;

const goImg = "brigadecore/go-tools:v0.1.0";
const kanikoImg = "brigadecore/kaniko:v0.2.0";
const helmImg = "brigadecore/helm-tools:v0.1.0";
const localPath = "/workspaces/brigade-github-gateway";

// MakeTargetJob is just a job wrapper around a make target.
class MakeTargetJob extends Job {
  constructor(target, img, e, env) {
    super(target, img);
    this.mountPath = localPath;
    this. env = env || {};
    this.env["SKIP_DOCKER"] = "true";
    const matchStr = e.revision.ref.match(releaseTagRegex);
    if (matchStr) {
      this.env["VERSION"] = Array.from(matchStr)[1];
    }
    this.tasks = [
      `cd ${localPath}`,
      `make ${target}`
    ];
  }
}

// PushImageJob is a specialized job type for publishing Docker images.
class PushImageJob extends MakeTargetJob {
  constructor(target, e, p) {
    super(target, kanikoImg, e, {
      "DOCKER_ORG": p.secrets.dockerhubOrg,
      "DOCKER_USERNAME": p.secrets.dockerhubUsername,
      "DOCKER_PASSWORD": p.secrets.dockerhubPassword
    });
  }
}

// A map of all jobs. When a check_run:rerequested event wants to re-run a
// single job, this allows us to easily find that job by name.
const jobs = {};

// Basic tests:

const testUnitJobName = "test-unit";
const testUnitJob = (e, p) => {
  return new MakeTargetJob(testUnitJobName, goImg, e);
}
jobs[testUnitJobName] = testUnitJob;

const lintJobName = "lint";
const lintJob = (e, p) => {
  return new MakeTargetJob(lintJobName, goImg, e);
}
jobs[lintJobName] = lintJob;

// Docker images:

const buildReceiverJobName = "build-receiver";
const buildReceiverJob = (e, p) => {
  return new MakeTargetJob(buildReceiverJobName, kanikoImg, e);
}
jobs[buildReceiverJobName] = buildReceiverJob;

const pushReceiverJobName = "push-receiver";
const pushReceiverJob = (e, p) => {
  return new PushImageJob(pushReceiverJobName, e, p);
}
jobs[pushReceiverJobName] = pushReceiverJob;

const buildMonitorJobName = "build-monitor";
const buildMonitorJob = (e, p) => {
  return new MakeTargetJob(buildMonitorJobName, kanikoImg, e);
}
jobs[buildMonitorJobName] = buildMonitorJob;

const pushMonitorJobName = "push-monitor";
const pushMonitorJob = (e, p) => {
  return new PushImageJob(pushMonitorJobName, e, p);
}
jobs[pushMonitorJobName] = pushMonitorJob;

// Helm chart:

const lintChartJobName = "lint-chart";
const lintChartJob = (e, p) => {
  return new MakeTargetJob(lintChartJobName, helmImg, e);
}
jobs[lintChartJobName] = lintChartJob;

const publishChartJobName = "publish-chart";
const publishChartJob = (e, p) => {
  return new MakeTargetJob(publishChartJobName, helmImg, e, {
    "HELM_REGISTRY": p.secrets.helmRegistry || "ghcr.io",
    "HELM_ORG": p.secrets.helmOrg,
    "HELM_USERNAME": p.secrets.helmUsername,
    "HELM_PASSWORD": p.secrets.helmPassword
  });
}
jobs[publishChartJobName] = publishChartJob;


// Run the entire suite of tests, builds, etc. concurrently WITHOUT publishing
// anything initially. If EVERYTHING passes AND this was a push (merge,
// presumably) to the master branch, then run jobs to publish "edge" images.
function runSuite(e, p) {
  // Important: To prevent Promise.all() from failing fast, we catch and
  // return all errors. This ensures Promise.all() always resolves. We then
  // iterate over all resolved values looking for errors. If we find one, we
  // throw it so the whole build will fail.
  //
  // Ref: https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Promise/all#Promise.all_fail-fast_behaviour
  return Promise.all([
    // Basic tests:
    run(e, p, testUnitJob(e, p)).catch((err) => { return err }),
    run(e, p, lintJob(e, p)).catch((err) => { return err }),
    // Docker images:
    run(e, p, buildReceiverJob(e, p)).catch((err) => { return err }),
    run(e, p, buildMonitorJob(e, p)).catch((err) => { return err }),
    // Helm chart:
    run(e, p, lintChartJob(e, p)).catch((err) => { return err })
  ]).then((values) => {
    values.forEach((value) => {
      if (value instanceof Error) throw value;
    });
  }).then(() => {
    if (e.revision.ref == "master") {
      // Push "edge" images.
      Promise.all([
        run(e, p, pushReceiverJob(e, p)).catch((err) => { return err }),
        run(e, p, pushMonitorJob(e, p)).catch((err) => { return err }),
      ]).then((values) => {
        values.forEach((value) => {
          if (value instanceof Error) throw value;
        }); 
      })
    }
  });
}

// run the specified job, sandwiched between two other jobs to report status
// via the GitHub checks API.
function run(e, p, job) {
  console.log("Check requested");
  var check = new Check(e, p, job, `https://brigadecore.github.io/kashti/builds/${e.buildID}`);
  return check.run();
}

// Either of these events should initiate execution of the entire test suite.
events.on("check_suite:requested", runSuite);
events.on("check_suite:rerequested", runSuite);

// These events MAY indicate that a maintainer has expressed, via a comment,
// that the entire test suite should be run.
events.on("issue_comment:created", (e, p) => Check.handleIssueComment(e, p, runSuite));
events.on("issue_comment:edited", (e, p) => Check.handleIssueComment(e, p, runSuite));

// This event indicates a specific job is to be re-run.
events.on("check_run:rerequested", (e, p) => {
  const jobName = JSON.parse(e.payload).body.check_run.name;
  const job = jobs[jobName];
  if (job) {
    return run(e, p, job(e, p));
  }
  throw new Error(`No job found with name: ${jobName}`);
});

// Pushing new commits to any branch in github triggers a check suite. Such
// events are already handled above. Here we're only concerned with the case
// wherein a new TAG has been pushed-- and even then, we're only concerned with
// tags that look like a semantic version and indicate a formal release should
// be performed.
events.on("push", (e, p) => {
  const matchStr = e.revision.ref.match(releaseTagRegex);
  if (matchStr) {
    // This is an official release with a semantically versioned tag
    return Group.runAll([
      pushReceiverJob(e, p),
      pushMonitorJob(e, p)
    ])
    .then(() => {
      // All images built and published successfully, so build and publish the
      // rest...
      Group.runAll([
        publishChartJob(e, p)
      ]);
    });
  }
  console.log(`Ref ${e.revision.ref} does not match release tag regex (${releaseTagRegex}); not releasing.`);
});
