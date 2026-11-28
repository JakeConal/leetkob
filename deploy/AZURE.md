# PA#3 Azure deployment

The owner confirmed Toàn’s student-credit coverage on November 28, 2026 and
authorized completion of PA#3 in `jenkins_group`. A single VM was provisioned
successfully in Southeast Asia using `azure.json`, Canonical Ubuntu 24.04
image version `24.04.202611170`, `Standard_B2als_v2` and a 64 GiB Standard SSD.
The subscription spending limit was re-read from Azure and remains **On**.
No billing-plan upgrade was made. Credit balances and expiry are not fabricated:
the deployment account cannot read Toàn’s billing-profile credit summary.

Both `jenkins_group` and the standby `rg-leetkob` belong to the same shared
subscription. The signed-in user’s separate expired credit was not evidence of
that subscription’s balance. The original working checkout was preserved.

## Hosting configuration

`azure.json` prepares one Linux amd64 VM, a 64 GiB Standard SSD OS disk, a
Standard public IPv4 address, a NIC, a virtual network and a network security
group. Only HTTP and HTTPS are opened to the internet. Administration uses Azure
VM Run Command; SSH is not exposed. Password authentication is disabled.

The subscription policy currently permits `southeastasia`, `eastasia`,
`koreacentral`, `centralindia` and `japanwest`. The resource group's metadata
location is West US 2, but the VM must be in a permitted region. The template
defaults to Southeast Asia and allows only 4 GiB VM sizes `Standard_B2als_v2`
and `Standard_B2s`; capacity and subscription eligibility must be checked in
the selected region before deployment.

Observed Southeast Asia retail rates on November 28, 2026:

| Item                        |            Rate | Planning cost for 730 hours |
| --------------------------- | --------------: | --------------------------: |
| Linux B2als v2              | USD 0.0472/hour |                   USD 34.46 |
| E6 LRS Standard SSD, 64 GiB |  USD 4.80/month |                    USD 4.80 |
| Standard public IPv4        |  USD 0.005/hour |                    USD 3.65 |
| Baseline total              |                 |         **USD 42.91/month** |

Disk operations, outbound traffic and any applicable additional charges are
excluded. B2s instead of B2als v2 raises the baseline to USD 46.99/month.
These are estimates from the [Azure retail API][retail], not incurred charges
or a guarantee of student-credit eligibility. No reservation, paid marketplace
image, support plan, registry, managed database or domain purchase is planned.

The existing subscription-wide `leetkob-free-tier-alerts` budget is USD 50 per
calendar month, with actual alerts at USD 10/25/40/50 and a forecast alert at USD 50. The subscription spending limit remains On. A budget sends notifications;
it is not an automatic USD 50 cutoff or a live credit-balance check.

## Application images and isolation

Build from the release checkout:

```bash
docker build -t leetkob-cpp17:pa3 apps/backend/sandbox/cpp17
docker build -f apps/backend/checker/Dockerfile -t leetkob-local-checker:pa3 apps/backend
docker build -f scripts/local-analyzer.Dockerfile -t leetkob-local-analyzer:pa3 apps/backend
docker compose --env-file /etc/leetkob/deploy.env -f deploy/docker-compose.yml build api worker web provision
```

The `local` analyzer image names are fixed names already used by the current
worker; they are also needed on the release host. Requalify the sandbox and
checker containers on the selected host before accepting submissions.

The trusted worker now contains the Docker client, not a compiler or native
parser. `WORKER_ISOLATED_ANALYZER=1` sends parsing to the existing isolated
analyzer controller. Only the trusted worker receives the host Docker socket;
student and analyzer containers receive no socket, host binds, application
environment or credentials. Current main uses `docker cp` for task inputs, so
the earlier same-path work-directory bind is no longer needed.

The base configuration selects `MODEL_OFF=1`. The live overlay and
[private NIM setup](NIM.md) are available in PR #50 but activation remains pending. Hard rules use actual
isolated libclang analysis; semantic findings remain unsure for TA review. No
provider key is installed and this configuration makes no live semantic model
request. The committed first real evaluation remains separate evidence in
`evaluation/dev-run/`.

Keep the completed runtime configuration outside the source checkout, for
example `/etc/leetkob/deploy.env`, with mode 0600. Set `WEB_ORIGIN` and
`PUBLIC_HOST` to the actual Azure DNS hostname once it exists, `DOCKER_GID` to
the numeric socket group, and supply only synthetic assessment passwords.
Never commit that file or print a rendered Compose configuration containing it.

Start or recover the authorized deployment:

```bash
docker compose --env-file /etc/leetkob/deploy.env -f deploy/docker-compose.yml up -d --wait
docker compose --env-file /etc/leetkob/deploy.env -f deploy/docker-compose.yml --profile tools run --rm provision
docker compose --env-file /etc/leetkob/deploy.env -f deploy/docker-compose.yml --profile public up -d caddy
```

Provisioning creates five synthetic accounts and two classes, without seeding
the marking lab or resetting existing passwords. The application remains the
entry point for lab creation and submissions. Caddy terminates HTTPS; only its
80/443 ports are public. API, web and database host ports remain loopback-only.
The Azure public IP's DNS label avoids purchasing a domain.

## Local verification

The candidate images built successfully and PostgreSQL/API/worker/web became
healthy using the isolated `leetkob-pa3-release` Compose project. Its ports are
loopback-only: web 3330, API 8186 and PostgreSQL 5546. The original development
checkout and unrelated running applications were preserved.

The HTTP smoke exercised the real image stack with five synthetic accounts:
compliant merge sort AC/pass, forbidden-sort AC/violation with evidence,
injection-comment AC/violation, WA and CE with checker bypass, concealed
cross-class and other-student access, student-safe result fields, sample-only
test projections, and acceptance while the worker was stopped followed by
successful judging after restart. This is local HTTP evidence, not a public
deployment or browser recording.

The first native browser attempt could not start: the installed headless Chromium lacks the
system library `libnspr4.so`. No browser interaction or screenshot success is
claimed. This failed attempt was superseded by the isolated Playwright browser used for the public recording.

Root lint, formatting, type checking, security and production image builds
passed. Root tests passed with a dedicated disposable PostgreSQL instance and
`GOFLAGS=-p=1`; package scheduling was serialized after concurrent database
cleanup exceeded its deadline. The race detector, concurrency scenarios,
assertions, sandbox controls and expected labels were unchanged. An earlier
run failed because the machine's default PostgreSQL port belongs to another
application; that failure is not hidden as a successful integration run.

## Public service

The real service first passed public HTTPS verification at
`2026-11-28T01:57:30Z`:
<https://leetkob-pa3-21ae08a0.southeastasia.cloudapp.azure.com/live>.
Caddy obtained a valid Let's Encrypt certificate for the actual Azure hostname;
HTTPS negotiation and the secure/HttpOnly session attributes were checked.
The subscription was re-read after provisioning: Enabled, spending limit On,
USD 50 monthly budget and every configured actual/forecast alert enabled.

The public HTTP smoke passed all five fixture scenarios and concealed other-class
and other-student reads. Worker stop/submit/restart accepted queued work and then
judged it; PostgreSQL contains exactly one verdict set, three test results and
one judge job for that submission. See the actual reports in
[`../docs/evidence/pa3-azure/`](../docs/evidence/pa3-azure/README.md).

[retail]: https://learn.microsoft.com/en-us/rest/api/cost-management/retail-prices/azure-retail-prices

## Actual-host qualification and security review

The VM passed every test in `go test -count=1 -v ./internal/sandbox/`, including
all nine live attacks and the disabled-control negative tests, without a skip.
`TestImageMatchesTeamCaseLabels` passed on the same host with model requests off.
See [`../docs/evidence/pa3-azure/sandbox.txt`](../docs/evidence/pa3-azure/sandbox.txt),
[`checker-image.txt`](../docs/evidence/pa3-azure/checker-image.txt) and
[`host.json`](../docs/evidence/pa3-azure/host.json). The host runs Docker 29.10.1;
the digest-pinned worker client was verified against it. The first Run Command
bootstrap stopped before testing because its noninteractive environment lacked a
Go build cache; explicit `GOCACHE` and `GOPATH` paths fixed the setup. No test or
sandbox setting was changed.

Azure reports the OS disk encryption as `EncryptionAtRestWithPlatformKey`.
Public disk access was disabled and `networkAccessPolicy` set to `DenyAll`;
these properties control export, not the VM's attached-disk access. Reapply after
recreating the disk:

```bash
az disk update -g jenkins_group -n disk-leetkob-pa3-os \
  --network-access-policy DenyAll --public-network-access Disabled
```

The VM has no managed identity, Azure role assignments, model key or cloud
credential installed by the release. It needs no Azure API access; administration
uses the Azure control-plane Run Command agent. Only TCP 80/443 are allowed
inbound. HTTPS sessions remain secure/HttpOnly and database/API/web ports are
bound to loopback. Docker starts at boot and services restart unless stopped.

SonarCloud flags the ARM template's omitted managed identity and missing
customer-managed disk encryption/host encryption. These findings remain open for
review; this release does not disable the rules or claim that Sonar is green.
Azure's platform-managed encryption is enabled by default, as documented in
[managed disk encryption](https://learn.microsoft.com/en-us/azure/virtual-machines/disk-encryption).
Encryption at host is a separate option and is not claimed as enabled. Adding
an unused identity or a paid key-management service merely to quiet the scanner
would change the deployment's permissions or cost without a demonstrated need.

## Operations and retention

Use Azure Run Command with commands below; do not print `/etc/leetkob/deploy.env`
or a rendered Compose configuration. Its generated database credential stays on
the host outside the source checkout and build contexts.

```bash
cd /opt/leetkob
docker compose --env-file /etc/leetkob/deploy.env -f deploy/docker-compose.yml ps
curl -fsS http://127.0.0.1:8080/healthz
docker compose --env-file /etc/leetkob/deploy.env -f deploy/docker-compose.yml restart worker
```

For an application update, fetch and check out a reviewed immutable revision,
rebuild the three application images sequentially and run `up -d --wait`.
Preserve the PostgreSQL and Caddy volumes. To roll back an application image,
check out the preceding tested revision and rebuild/up; do not run `down -v` or
roll back database migrations automatically. No destructive migration was used.

Keep the service running until PA#4 marks, while monitoring the owner’s real
credit expiry and the USD 50 budget notifications. No future uptime or credit
balance can be guaranteed by a snapshot. The actual first deployment date is
November 28, before the updated November 29, 22:00 Asia/Saigon deadline.
