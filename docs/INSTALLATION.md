# Installation

## Prerequisites

* A GitHub account
* A Kubernetes cluster:
  * For which you have the `admin` cluster role.
  * That is already running Brigade v2.2.0 or greater.
  * That is capable of provisioning a _public IP address_ for a service of type
    `LoadBalancer`.

    > ⚠️&nbsp;&nbsp;This means you won't have much luck running the gateway
    > locally in the likes of [KinD](https://kind.sigs.k8s.io/) or
    > [minikube](https://minikube.sigs.k8s.io/docs/) unless you're able and
    > willing to create port forwarding rules on your router or make use of a
    > service such as [ngrok](https://ngrok.com/). Both of these are beyond
    > the scope of this documentation.
* `kubectl`
* `helm`: Commands below require `helm` 3.7.0+.
* `brig`: The Brigade CLI. Commands below require `brig` 2.0.0+.

## Planning

Excluding advanced scenarios wherein you utilize a Kubernetes
[ingress controller](https://kubernetes.io/docs/concepts/services-networking/ingress-controllers/)
as a reverse proxy for this gateway (which we won't be covering here), there is
sometimes a minor dilemma involved in the installation process. Namely, to
create a [GitHub App](https://docs.github.com/en/developers/apps/about-apps)
that will send webhooks to your gateway, you will need the public IP or fully
qualified hostname where your gateway will be addressable on the internet. If
you are planning to simply use the public IP, it is probable you do not _know_
that value yet and won't until after the gateway is deployed. However,
installing the gateway requires configuration that is obtained during the
creation of the GitHub App. This presents a sort of
["chicken and egg"](https://en.wikipedia.org/wiki/Chicken_or_the_egg) problem.
Knowing this complication exists and planning for it in advance can streamline
the installation process.

Our best recommendation is to sidestep the problem altogether by choosing an
available DNS hostname for the gateway in advance. Use that address when
creating your GitHub App, but defer creation of the actual DNS record until
after the gateway is deployed and its public IP becomes known.

If you are unable to follow the recommendation above and need to use the public
IP directly, plan to initially create the GitHub App using a _placeholder_ for
the gateway's address. In this case, the GitHub App's configuration can be
revisited and corrected after the gateway is deployed.

## Create a GitHub App

A [GitHub App](https://docs.github.com/en/developers/apps/about-apps) is a
special kind of trusted entity that is "installable" into GitHub repositories to
enable integrations with third-parties. In this case, Brigade, via your gateway,
will be the third-party.

> ⚠️&nbsp;&nbsp;This gateway can support multiple GitHub Apps, but these
> instructions walk you through the steps for setting up just one.

1. Visit https://github.com/settings/apps/new.

1. Choose a _globally unique_ __GitHub App name__. When you submit the form,
   you will be informed if the name you selected is unavailable.

1. Set the __Homepage URL__ to
   `https://github.com/brigadecore/brigade-github-gateway`. Any URL will work,
   but since this is where GitHub users will be directed if they wish to know
   more about your GitHub App, something informative is best.

1. Set the __Webhook URL__ to `https://<hostname or public address>/events`. Per
   the [planning](#planning) section, this value can be a temporary placeholder
   if necessary.

1. Set the __Webhook Secret__ to a complex string. It is, fundamentally, a
   password, so make it strong. If you're personally in the habit of using a
   password manager and it can generate strong passwords for you, consider using
   that. Make a note of this __shared secret__. You will be using this value
   again in another step.

1. Under the __Subscribe to events__ section, select any events you wish to
   propagate to the gateway in the form of a webhook. 
    
   > ⚠️&nbsp;&nbsp;Selecting additional permissions in the __Repository
   > permissions__ section adds additional options to the menu of subscribable
   > events.
   >
   > For the example in the [README](../README.md), you would require the
   > __Watching__ permission and a subscription to the __Watch__ event.

1. Under __Where can this GitHub App be installed?__ select __Only This account__
   to constrain your GitHub App to being installed only by repositories in your
   own account or organization.

1. Submit the form.

1. Find the __App ID__ field on the confirmation page and note its value. You
   will be using this value again in another step.

1. Under the `Private keys` section of this page, click `Generate a private
   key`. After generating, _immediately_ download the new key.

   > ⚠️&nbsp;&nbsp;This is your only opportunity to download the private key, as
   > GitHub will only save the public half of the key. You will be using this
   > key in another step.

## Create a Service Account for the Gateway

> ⚠️&nbsp;&nbsp;To proceed beyond this point, you'll need to be logged into
> Brigade as the "root" user (not recommended) or (preferably) as a user with
> the `ADMIN` role. Further discussion of this is beyond the scope of this
> documentation. Please refer to Brigade's own documentation.

1. Using the `brig` CLI, create a service account for the gateway to use:

   ```console
   $ brig service-account create \
       --id brigade-github-gateway \
       --description brigade-github-gateway
   ```

1. Make note of the __token__ returned. This value will be used in another step.

   > ⚠️&nbsp;&nbsp;This is your only opportunity to access this value, as
   > Brigade does not save it.

1. Authorize this service account to read all events and to create new ones:

   ```console
   $ brig role grant READER \
       --service-account brigade-github-gateway

   $ brig role grant EVENT_CREATOR \
       --service-account brigade-github-gateway \
       --source brigade.sh/github
   ```

   > ⚠️&nbsp;&nbsp;The `--source brigade.sh/github` option specifies that this
   > service account can be used _only_ to create events having a value of
   > `brigade.sh/github` in the event's `source` field. This is a security
   > measure that prevents the gateway from using this token for impersonating
   > other gateways.

## Install the Gateway

> ⚠️&nbsp;&nbsp;be sure you are using
> [Helm 3.7.0](https://github.com/helm/helm/releases/tag/v3.7.0) or greater and
> enable experimental OCI support:
>
> ```console
>  $ export HELM_EXPERIMENTAL_OCI=1
>  ```

1. As this gateway requires some specific configuration to function properly,
   we'll first create a values file containing those settings. Use the following
   command to extract the full set of configuration options into a file you can
   modify:

   ```console
   $ helm inspect values oci://ghcr.io/brigadecore/brigade-github-gateway \
       --version v1.0.0 > ~/brigade-github-gateway-values.yaml
   ```

1. Edit `~/brigade-github-gateway-values.yaml`, making the following changes:

   * `brigade.apiAddress`: Set this to the address of the Brigade API server,
     beginning with `https://`.

   * `brigade.apiToken`: Set this to the service account token obtained when you
     created the Brigade service account for this gateway.

   * `github.apps`: Specify the details of your GitHub App(s), including:

     * `appID`: Set this to the App ID obtained when you created your GitHub
       App.

     * `apiKey`: Set this to the private key you created and downloaded after
       creating your GitHub App. The value should begin with
       `-----BEGIN RSA PRIVATE KEY-----` and end with
       `-----END RSA PRIVATE KEY-----`. All line breaks should be preserved
       and the beginning of each line should be indented exactly four spaces.

    * `sharedSecret`: Set this to the shared secret you chose when you created
      your GitHub App.

   * `receiver.host`: If you chose an available DNS hostname for your gateway
      when setting up your GitHub App, use that value. If you planned to use the
      public IP instead and used a placeholder value when creating the GitHub
      App, then the value of this field is less important and can be left as is.

   * `receiver.service.type`: If you plan to enable ingress (advanced), you can
    leave this as its default -- `ClusterIP`. If you do not plan to enable
    ingress, you should change this value to `LoadBalancer`.

   > ⚠️&nbsp;&nbsp;By default, TLS will be enabled and a self-signed certificate
   > will be generated. While most clients would not trust such a certificate,
   > your GitHub App will, unless you explicitly indicated it should not
   > tolerate certificate errors when you created it.
   >
   > For a production-grade deployment you should explore the options available
   > for providing or provisioning a certificate signed by a trusted authority.
   > These options can be located under the `receiver.tls` and
   > `receiver.ingress.tls` sections of the values file.

1. Save your changes to `~/brigade-github-gateway-values.yaml`.

1. Use the following command to install the gateway:

   ```console
   $ helm install brigade-github-gateway \
       oci://ghcr.io/brigadecore/brigade-github-gateway \
       --version v1.0.0 \
       --create-namespace \
       --namespace brigade-github-gateway \
       --values ~/brigade-github-gateway-values.yaml \
       --wait \
       --timeout 300s
   ```

## Exposing the Gateway

In the [planning](#planning) section, we recommended selecting an available DNS
hostname as the address of your gateway. If you have done so, now is the time to
obtain the gateway's public IP address and create a DNS entry. If, instead, you
elected to use a placeholder value when creating your GitHub App, it is time to
revisit that configuration and replace it with the gateway's public IP address.
In either case, we need to obtain that address now.

If you overrode default configuration and set `service.type` to `LoadBalancer`,
use this command to find the gateway's public IP address:

```console
$ kubectl get svc brigade-github-gateway-receiver \
    --namespace brigade-github-gateway \
    --output jsonpath='{.status.loadBalancer.ingress[0].ip}'
```

If you overrode default configuration to enable support for an ingress
controller, you probably know what you're doing well enough to track down the
correct IP for that ingress controller without our help. 😉

With this public IP in hand, create an `A` record pointing your hostname to your
gateway's public IP or revisit your GitHub App's configuration and replace the
placeholder value in the __Webhook URL__ field.

> ⚠️&nbsp;&nbsp;Since it can be somewhat difficult to locate if you're not
> already very familiar with GitHub Apps, your GitHub App's configuration can be
> revisited by browsing to a URL of the form
> `https://github.com/settings/apps/<app name>`.

## Confirm Connectivity

Your GitHub App can be found on GitHub using a URL of the form
`https://github.com/settings/apps/<app name>`.

Browse to the __Advanced__ tab and check out the __Recent Deliveries__ section.
Here you can view webhooks that your GitHub App has recently attempted to
deliver to your new gateway. There shouldn't be many events displayed yet, but
there should be at least one `ping` that the GitHub App attempted to deliver to
the gateway when it was created. This should have failed since we set up the App
on GitHub's end _prior_ to installing the gateway on our cluster. Click
__Redeliver__. If re-delivery succeeds, you're all set!

If re-delivery failed, you can examine request and response headers and the JSON
payload to attempt to make some determination as to what has gone wrong.

Some likely problems include:

* Your A record in DNS is incorrect.

* DNS changes have not propagated.

* Your gateway is not listening on a public IP.

* The __Webhook URL__ you entered when configuring the GitHub App is incorrect.

* The gateway was not configured correctly using the GitHub App's __App ID__
  and __shared secret__.

## Install the App

Once again, your GitHub App can be found on GitHub using a URL of the form
`https://github.com/settings/apps/<app name>`.

Under the __Install App__ tab you can see all accounts and organizations into
whose repositories you can install your App. Click the gear icon next to the
desired account or organization and, under __Repository access__ choose __All
repositories__ OR __Only select repositories__ then specify which ones, and
click __Save__.
