# PA#3 Azure release preparation

Status on November 28, 2026: **locally verified; not deployed to Azure**.
The intended target is the existing `jenkins_group` in the shared Azure for
Students subscription. Both it and `rg-leetkob` belong to the same subscription;
changing resource groups does not change which billing profile funds usage.
The group tags now identify `jenkins_group` as active and `rg-leetkob` as
standby. This metadata change created no billable resource and changed no role.

The subscription's linked billing profile belongs to the teammate who owns the
shared subscription. Its credit summary is not readable by the deployment
account, even though that account has subscription Owner access. The owner's
active credit balance and expiry must be confirmed before creating resources.
The signed-in user's separate billing profile showed expired credit; it is not
evidence of the shared subscription's balance. No paid resources were created.

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

`MODEL_OFF=1` is fixed in this deployment configuration. Hard rules use actual
isolated libclang analysis; semantic findings remain unsure for TA review. No
provider key is installed and this configuration makes no live semantic model
request. The committed first real evaluation remains separate evidence in
`evaluation/dev-run/`.

Keep the completed runtime configuration outside the source checkout, for
example `/etc/leetkob/deploy.env`, with mode 0600. Set `WEB_ORIGIN` and
`PUBLIC_HOST` to the actual Azure DNS hostname once it exists, `DOCKER_GID` to
the numeric socket group, and supply only synthetic assessment passwords.
Never commit that file or print a rendered Compose configuration containing it.

After provisioning is authorized and credit coverage is confirmed:

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

The browser check could not start: the installed headless Chromium lacks the
system library `libnspr4.so`. No browser interaction or screenshot success is
claimed. Browser verification and the public fallback recording remain open.

Root lint, formatting, type checking, security and production image builds
passed. Root tests passed with a dedicated disposable PostgreSQL instance and
`GOFLAGS=-p=1`; package scheduling was serialized after concurrent database
cleanup exceeded its deadline. The race detector, concurrency scenarios,
assertions, sandbox controls and expected labels were unchanged. An earlier
run failed because the machine's default PostgreSQL port belongs to another
application; that failure is not hidden as a successful integration run.

## Still required for release

- Confirm the shared billing profile's active credits and expiry.
- Validate and provision the bounded template in an allowed region.
- Qualify isolation on the actual host and verify the HTTPS walkthrough and
  worker recovery there.
- Capture the actual at-most-five-minute public walkthrough; do not label the
  local smoke as that recording.
- Update PA3.md and the self-assessment with real public URL/accounts/evidence,
  run CI on the final `pa3` tag and verify the complete submission archive.
- Record the actual first deployment date. Availability since the November 25
  deadline cannot be claimed retrospectively.

[retail]: https://learn.microsoft.com/en-us/rest/api/cost-management/retail-prices/azure-retail-prices
