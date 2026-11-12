# Runzivo GitLab CI/CD component

Source bundle for a reusable GitLab CI job on Runzivo's managed Linux runners.
Catalog publication is pending. No published Catalog URL or release version is
assigned. A working integration does not establish GitLab partner verification.

Project description: Run GitLab CI jobs on managed Runzivo Linux runners using a
reusable component.

## Before using it

Where GitLab.com access is enabled for your Runzivo workspace, connect GitLab in
[Settings > Integrations](https://app.runzivo.dev/settings?tab=integrations), select
your project by name and choose **Enable Runzivo runners**. Complete that setup
before running a tagged job. The component does not create or register runners,
accept runner tokens or require the `rzv` CLI.

Read the [GitLab.com setup guide](https://runzivo.dev/docs/gitlab-com/) for current
availability, permissions and connection troubleshooting. Self-managed GitLab is
outside this bundle's supported integration scope.

## Components

### job

Creates one job, named `runzivo-test` by default, using the `runzivo` runner tag.
Pass your trusted commands as a native YAML array. Inputs are declared and
described in [templates/job.yml](templates/job.yml).

This local-include example works when this source bundle is at the root of the
pipeline's GitLab project. It is an example for the bundle's maintainer, not a
published Catalog installation command:

```yaml
include:
  - local: /templates/job.yml
    inputs:
      job-name: unit-tests
      stage: test
      script:
        - uname -a
        - ./scripts/test.sh

stages: [test]
```

The project supplies `scripts/test.sh`. The resulting job is
`runzivo-unit-tests`. Include the component again with a different `job-name` to
create another independent job. Choose names that do not collide with existing
pipeline jobs.

After a Catalog release exists, copy its generated `include: component` reference
and pin the exact release version or commit. Do not invent a namespace, use a
moving branch or treat this bundle's source path as a published component URL.

Runzivo's Custom executor runs scripts in its Linux VM. It does not accept
`image`, `services` or executor options. The component disables inheritance of
pipeline `default` settings so an inherited image or service does not change the
job. Put required setup commands in `script`, or add compatible job-level
configuration explicitly. Project CI/CD variables retain their ordinary GitLab
behavior. Store credentials in protected or masked variables, not component
inputs or repository files.

The tag selects the connected Runzivo runner. It does not select CPU, memory or
disk, change plan eligibility or alter billing. Do not substitute GitHub
`runs-on` size labels for this GitLab tag.

## Test the bundle

The bundled [.gitlab-ci.yml](.gitlab-ci.yml) includes the component twice from the
same project and commit. It checks the default job and a renamed job in a custom
stage, then verifies both result artifacts. This exercises native input handling,
job-name isolation, runner execution and ordinary artifact overrides. No account
credentials are printed or submitted through the component.

The pipeline is intended for a company-approved GitLab.com project with its own
Runzivo connection. It excludes merge requests from a different source project.
Do not use an unrelated or customer-owned project to test this bundle.

The release job uses GitLab's container-capable `saas-linux-small-amd64` runner
and a digest-pinned GitLab CLI image. It does not try to run a container image on
Runzivo's Custom executor. GitLab-hosted runner availability and compute-minute
rules apply to that release job.

Local syntax and result-checker checks do not prove GitLab compiled the
component or a live Runzivo job succeeded. Record the real pipeline and job
results after testing in the approved project.

## Maintainer publication checklist

1. Confirm the approved GitLab.com project and the license for public Catalog
   reuse. The existing proprietary license is preserved in
   [LICENSE.md](LICENSE.md); no open-source license is granted by this bundle.
2. Put this bundle's contents at that project's repository root, so `templates/`,
   `README.md`, `LICENSE.md`, `.gitlab-ci.yml` and `tests/` are in the expected
   locations. This nested GitHub directory is not itself a Catalog project.
3. Set the project description and public visibility. Connect that project to
   Runzivo through OAuth and run the branch pipeline. Confirm both component jobs
   and the artifact verification job pass.
4. With the project's Owner role, enable **CI/CD Catalog project** under
   **Settings > General > Visibility, project features, permissions**.
5. After tests pass, create a stable semantic-version tag such as `0.1.0`. The
   tag pipeline tests the component before its `release` job publishes the
   release. Do not replace that job with a direct Releases API call.
6. Open the Catalog entry, test its displayed include reference in a separate
   authorized project, and record its release URL and maintenance owner.

The bundled release rule accepts stable `MAJOR.MINOR.PATCH` tags without a `v`
prefix. It does not release prerelease tags. GitLab Partner listing and namespace
verification are separate applications, not consequences of publishing a release.

## References

- [GitLab component structure, testing and Catalog publication](https://docs.gitlab.com/ci/components/)
- [Typed CI/CD inputs](https://docs.gitlab.com/ci/inputs/)
- [Custom executor](https://docs.gitlab.com/runner/executors/custom/)
- [GitLab-hosted Linux runner tags and container support](https://docs.gitlab.com/ci/runners/hosted_runners/linux/)
- [Release jobs](https://docs.gitlab.com/user/project/releases/release_cicd_examples/)
- [Third-party CI tooling](THIRD_PARTY_NOTICES.md)

## License and maintenance

The bundle retains the repository's existing proprietary license. Public Catalog
redistribution and customization rights require the company's license decision
before publication. The CI release tool has its own license, linked in the
third-party notices. Report Runzivo integration problems to
support@runzivo.dev. This repository does not accept outside contributions.
