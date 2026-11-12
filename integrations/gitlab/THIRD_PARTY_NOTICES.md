# Third-party CI tooling

The Catalog release job references GitLab CLI 1.124.0 through the official
`registry.gitlab.com/gitlab-org/cli` container image, pinned to manifest digest
`sha256:f6837b5aea075ca297281b554556d2fc4f8fb291ace6b48286790790dce622cd`.

GitLab CLI is MIT licensed. Its source and full license are published at:

- [GitLab CLI source at v1.124.0](https://gitlab.com/gitlab-org/cli/-/tree/v1.124.0)
- [GitLab CLI license at v1.124.0](https://gitlab.com/gitlab-org/cli/-/blob/v1.124.0/LICENSE)

The image runs only in the maintainer release job on a container-capable runner.
It is not included in the `rzv` executable or run by the component job.
