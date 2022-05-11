# Contributing Guide

The Brigade GitHub Gateway is an official extension of the Brigade project and
as such follows all of the practices and policies laid out in the main
[Brigade Contributor Guide](https://docs.brigade.sh/topics/contributor-guide/).
Anyone interested in contributing to this gateway should familiarize themselves
with that guide _first_.

The remainder of _this_ document only supplements the above with things specific
to this project.

## Running `make hack-kind-up`

As with the main Brigade repository, running `make hack-kind-up` in this
repository will utilize [ctlptl](https://github.com/tilt-dev/ctlptl) and
[KinD](https://kind.sigs.k8s.io/) to launch a local, development-grade
Kubernetes cluster that is also connected to a local Docker registry.

In contrast to the main Brigade repo, this cluster is not pre-configured for
building and running Brigade itself from source, rather it is pre-configured for
building and running _this gateway_ from source. Because Brigade is a logical
prerequisite for this gateway to be useful in any way, `make hack-kind-up` will
pre-install a recent, _stable_ release of Brigade into the cluster.

## Create a GitHub App

Create a [GitHub APP](https://docs.github.com/en/developers/apps/about-apps) as
described in [this section](docs/INSTALLATION.md#create-a-github-app) of the
installation docs. Use a placeholder for the __Webhook URL__ field. This
configuration will be revisited later.

## Running `tilt up`

As with the main Brigade repository, running `tilt up` will build and deploy
project code (the gateway, in this case) from source.

For the gateway to successfully communicate with the Brigade instance in your
local, development-grade cluster, you will need to execute the following steps
_before_ running `tilt up`:

1. Log into Brigade:

   ```shell
   $ brig login -k -s https://localhost:31600 --root
   ```

   The root password is `F00Bar!!!`.

1. Create a service account for the gateway:

   ```shell
   $ brig service-account create \
       --id github-gateway \
       --description github-gateway
   ```

1. Copy the token returned from the previous step and export it as the
   `BRIGADE_API_TOKEN` environment variable:

   ```shell
   $ export BRIGADE_API_TOKEN=<token from previous step>
   ```

1. Grant the service account permission to create events:

   ```shell
   $ brig role grant EVENT_CREATOR \
     --service-account github-gateway \
     --source brigade.sh/github
   ```

   > ⚠️&nbsp;&nbsp;Contributions that automate the creation and configuration of
   > the service account setup are welcome.

1. Edit the `github.apps` section of `charts/brigade-github-gateway/values.yaml`
   with your `appID`, `apiKey`, and `sharedSecret`. Refer to instructions in
   [this section](docs/INSTALLATION.md#create-a-github-app) of the installation
   docs for more information.

   > ⚠️&nbsp;&nbsp;Take care not to include modifications to the `values.yaml`
   > file in any PRs you open.

You can then run `tilt up` to build and deploy this gateway from source.

## Receiving Events from GitHub

Making the gateway that runs in your local, development-grade Kubernetes cluster
visible to GitHub such that it can successfully deliver webhooks can be
challenging. To help ease this process, our `Tiltfile` has built-in support for
exposing your local gateway using [ngrok](https://ngrok.com/). To take advantage
of this:

1. [Sign up](https://dashboard.ngrok.com/signup) for a free ngrok account.

1. Follow ngrok
   [setup & installation instructions](https://dashboard.ngrok.com/get-started/setup)

1. Set the environment variable `ENABLE_NGROK_EXTENSION` to a value of `1`
   _before_ running `tilt up`.

1. After running `tilt up`, the option should become available in the Tilt UI at
  `http://localhost:10350/` to expose the gateway using ngrok. After going so,
   the applicable ngrok URL will be displayed in the gateway's logs in the Tilt
   UI.

1. Revisit your GitHub App configuration at
   `https://github.com/settings/apps/<app name>` and replace the placeholder
   value in the __Webhook URL__ field with the ngrok URL from the previous step.

> ⚠️&nbsp;&nbsp;We cannot guarantee that ngrok will work in all environments,
> especially if you are behind a corporate firewall.
